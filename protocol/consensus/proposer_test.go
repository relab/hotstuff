package consensus_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

func check(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func wireUpProposer(
	t *testing.T,
	depsCore *wiring.Core,
	depsSecurity *wiring.Security,
	sender modules.Sender,
	commandCache *clientpb.CommandCache,
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
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		hsProtocol,
		depsSecurity.Authority(),
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
	t.Skip() // TODO(AlanRostem): fix test
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
			t.Fatal(err)
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
	command := &clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("testing data here"),
	}
	commandCache := clientpb.NewCommandCache()
	commandCache.Add(command)
	proposer := wireUpProposer(t, replica0.depsCore, replica0.depsSecurity, replica0.sender, commandCache, list)
	// block := testutil.CreateBlock(t, replica0.depsSecurity.Authority())
	// qc := testutil.CreateQC(t, block, signers)
	proposal, err := proposer.CreateProposal(1, hotstuff.GetGenesis().QuorumCert(), hotstuff.NewSyncInfo())
	if err != nil {
		t.Error(err)
	}
	proposer.Propose(&proposal)
	messages := replica0.sender.MessagesSent()
	if len(messages) != 1 {
		t.Fatal("expected at least one message to be sent by proposer")
	}

	msg := messages[0]
	if _, ok := msg.(hotstuff.ProposeMsg); !ok {
		t.Fatal("expected message to be hotstuff.ProposeMsg")
	}

	proposeMsg := msg.(hotstuff.ProposeMsg)
	if proposeMsg.ID != 1 || !bytes.Equal(proposeMsg.Block.ToBytes(), proposal.Block.ToBytes()) {
		t.Fatal("incorrect propose message data")
	}
}
