package synchronizer

import (
	"testing"

	"cuelang.org/go/pkg/time"
	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/clique"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

const cryptoName = ecdsa.ModuleName

func wireUpSynchronizer(t *testing.T) (
	*wiring.Core,
	*wiring.Security,
	*clientpb.CommandCache,
	*protocol.ViewStates,
	*wiring.Consensus,
	*Synchronizer,
) {
	id := hotstuff.ID(1)
	pk := testutil.GenerateECDSAKey(t)
	depsCore := wiring.NewCore(id, "test", pk)
	sender := testutil.NewMockSender(id)
	depsSecurity, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		cryptoName,
	)
	if err != nil {
		t.Fatal(err)
	}
	leaderRotation := fixedleader.New(id)
	consensusRules := chainedhotstuff.New(
		depsCore.Logger(),
		depsSecurity.BlockChain(),
	)
	viewStates, err := protocol.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache(1)
	committer := consensus.NewCommitter(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecurity.BlockChain(),
		viewStates,
		consensusRules,
	)
	votingMachine := votingmachine.New(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		viewStates,
	)
	depsConsensus := wiring.NewConsensus(
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		commandCache,
		committer,
		consensusRules,
		leaderRotation,
		clique.New(
			depsCore.RuntimeCfg(),
			votingMachine,
			leaderRotation,
			sender,
		),
	)
	synchronizer := New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.Authority(),
		leaderRotation,
		viewduration.NewFixed(1000*time.Nanosecond),
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		viewStates,
		sender,
	)

	return depsCore, depsSecurity, commandCache, viewStates, depsConsensus, synchronizer
}

func makeSigners(t *testing.T, leaderCfg *core.RuntimeConfig, leaderAuth *cert.Authority) []*cert.Authority {
	signers := make([]*cert.Authority, 0)
	signers = append(signers, leaderAuth)
	const n = 4
	for i := range n - 1 {
		id := hotstuff.ID(i + 2)
		pk := testutil.GenerateECDSAKey(t)
		core := wiring.NewCore(id, "test", pk)
		leaderCfg.AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: pk.Public(),
		})
		security, err := wiring.NewSecurity(
			core.EventLoop(),
			core.Logger(),
			core.RuntimeCfg(),
			testutil.NewMockSender(id),
			cryptoName,
		)
		if err != nil {
			t.Fatal(err)
		}
		signers = append(signers, security.Authority())
	}
	return signers
}

func TestAdvanceViewQC(t *testing.T) {
	const n = 4
	depsCore,
		depsSecurity,
		commandCache,
		viewStates,
		depsConsensus,
		synchronizer := wireUpSynchronizer(t)

	blockchain := depsSecurity.BlockChain()
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()),
		&clientpb.Batch{
			Commands: []*clientpb.Command{
				{
					Data: []byte("foo"),
				},
			},
		},
		1,
		1,
	)
	blockchain.Store(block)
	signers := makeSigners(t, depsCore.RuntimeCfg(), depsSecurity.Authority())
	qc := testutil.CreateQC(t, block, signers)
	proposer := depsConsensus.Proposer()
	commandCache.Add(&clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("bar"),
	})
	proposal, err := proposer.CreateProposal(1, viewStates.HighQC(), viewStates.SyncInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := proposer.Propose(&proposal); err != nil {
		t.Fatal(err)
	}

	synchronizer.advanceView(hotstuff.NewSyncInfo().WithQC(qc))

	if viewStates.View() != 2 {
		t.Errorf("wrong view: expected: %d, got: %d", 2, viewStates.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	depsCore,
		depsSecurity,
		commandCache,
		viewStates,
		depsConsensus,
		synchronizer := wireUpSynchronizer(t)
	signers := makeSigners(t, depsCore.RuntimeCfg(), depsSecurity.Authority())
	tc := testutil.CreateTC(t, 1, depsSecurity.Authority(), signers)

	proposer := depsConsensus.Proposer()
	commandCache.Add(&clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("bar"),
	})
	proposal, err := proposer.CreateProposal(1, viewStates.HighQC(), viewStates.SyncInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := proposer.Propose(&proposal); err != nil {
		t.Fatal(err)
	}
	synchronizer.advanceView(hotstuff.NewSyncInfo().WithTC(tc))

	if viewStates.View() != 2 {
		t.Errorf("wrong view: expected: %d, got: %d", 2, viewStates.View())
	}
}
