package consensus_test

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

func check(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}

func wireUpProposer(
	t *testing.T,
	depsCore *wiring.Core,
	depsSecurity *wiring.Security,
	sender modules.Sender,
	commandCache *clientpb.Cache,
	list moduleList,
) *consensus.Proposer {
	t.Helper()
	consensusRules, err := wiring.NewConsensusRules(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		list.consensusRules,
	)
	check(t, err)
	committer := committer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecurity.BlockChain(),
		consensusRules,
	)
	leaderRotation, err := wiring.NewLeaderRotation(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		committer,
		viewduration.NewParams(1, 100*time.Millisecond, 0, 2),
		list.leaderRotation,
		consensusRules.ChainLength(),
	)
	check(t, err)
	viewStates, err := consensus.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	check(t, err)
	hsProtocol := consensus.NewHotStuff(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		viewStates,
		leaderRotation,
		sender,
	)
	voter := consensus.NewVoter(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		hsProtocol,
		depsSecurity.Authority(),
		commandCache,
		committer,
	)
	return consensus.NewProposer(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		hsProtocol,
		voter,
		commandCache,
		committer,
	)
}

type replica struct {
	depsCore     *wiring.Core
	depsSecurity *wiring.Security
	sender       *testutil.MockSender
}

func TestPropose(t *testing.T) {
	// TODO(AlanRostem): put this in some test data
	list := moduleList{
		consensusRules: chainedhotstuff.ModuleName,
		leaderRotation: fixedleader.ModuleName,
		cryptoBase:     ecdsa.ModuleName,
	}

	const n = 4
	replicas := make([]replica, 0)
	signers := make([]*cert.Authority, 0)
	for i := range n {
		id := hotstuff.ID(i + 1)
		depsCore := wiring.NewCore(id, "test", testutil.GenerateECDSAKey(t))
		sender := testutil.NewMockSender(depsCore.RuntimeCfg().ID())
		depsSecurity, err := wiring.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			list.cryptoBase,
		)
		if err != nil {
			t.Error(err)
		}
		signers = append(signers, depsSecurity.Authority())
		replicas = append(replicas, replica{
			depsCore:     depsCore,
			depsSecurity: depsSecurity,
			sender:       sender,
		})
	}
	replica0 := replicas[0]
	// add the blockchains to the proposer's mock sender
	for _, other := range replicas {
		replica0.sender.AddBlockChain(other.depsSecurity.BlockChain())
	}
	commandCache := clientpb.New()
	commandCache.Add(&clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("testing data here"),
	})
	proposer := wireUpProposer(t, replica0.depsCore, replica0.depsSecurity, replica0.sender, commandCache, list)
	block := testutil.CreateBlock(t, replica0.depsSecurity.Authority())
	qc := testutil.CreateQC(t, block, signers)
	proposal, err := proposer.CreateProposal(1, qc, hotstuff.NewSyncInfo())
	if err != nil {
		t.Error(err)
	}
	proposer.Propose(&proposal)
}
