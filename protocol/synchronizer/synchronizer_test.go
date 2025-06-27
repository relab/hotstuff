package synchronizer

import (
	"testing"

	"cuelang.org/go/pkg/time"
	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/wiring"
)

func wireUpSynchronizer(
	t *testing.T,
	essentials *testutil.Essentials,
	commandCache *clientpb.CommandCache,
	viewStates *protocol.ViewStates,
) (*Synchronizer, *consensus.Proposer) {
	t.Helper()
	leaderRotation := leaderrotation.NewFixed(1)
	consensusRules := rules.NewChainedHotStuff(
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
	)
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		viewStates,
	)
	depsConsensus := wiring.NewConsensus(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		commandCache,
		consensusRules,
		leaderRotation,
		viewStates,
		comm.NewClique(
			essentials.RuntimeCfg(),
			votingMachine,
			leaderRotation,
			essentials.MockSender(),
		),
	)
	synchronizer := New(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.Authority(),
		leaderRotation,
		viewduration.NewFixed(1000*time.Nanosecond),
		NewSimple(essentials.RuntimeCfg(), essentials.Authority()),
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		viewStates,
		essentials.MockSender(),
	)
	return synchronizer, depsConsensus.Proposer()
}

func TestAdvanceViewQC(t *testing.T) {
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]
	viewStates, err := protocol.NewViewStates(
		subject.BlockChain(),
		subject.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache(1)
	synchronizer, proposer := wireUpSynchronizer(t, subject, commandCache, viewStates)

	blockchain := subject.BlockChain()
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()),
		&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte("foo")}}},
		1,
		1,
	)
	blockchain.Store(block)

	signers := set.Signers()
	qc := testutil.CreateQC(t, block, signers...)
	for i := range 2 {
		// adding multiple commands so the next call CreateProposal
		// in advanceView doesn't block
		commandCache.Add(&clientpb.Command{
			ClientID:       1,
			SequenceNumber: uint64(i + 1),
			Data:           []byte("bar"),
		})
	}
	proposal, err := proposer.CreateProposal(viewStates.SyncInfo())
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
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]
	viewStates, err := protocol.NewViewStates(
		subject.BlockChain(),
		subject.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache(1)
	synchronizer, proposer := wireUpSynchronizer(t, subject, commandCache, viewStates)

	signers := set.Signers()
	tc := testutil.CreateTC(t, 1, signers)
	for i := range 2 {
		// adding multiple commands so the next call CreateProposal
		// in advanceView doesn't block
		commandCache.Add(&clientpb.Command{
			ClientID:       1,
			SequenceNumber: uint64(i + 1),
			Data:           []byte("bar"),
		})
	}
	proposal, err := proposer.CreateProposal(viewStates.SyncInfo())
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
