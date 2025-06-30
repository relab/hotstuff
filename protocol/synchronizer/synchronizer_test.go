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
		NewFixedDuration(1000*time.Nanosecond),
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

func TestAdvanceView(t *testing.T) {
	const (
		S = false // simple timeout rule
		A = true  // aggregate timeout rule
		F = false
		T = true
	)
	tests := []struct {
		name           string
		tr, qc, tc, ac bool
		firstSignerIdx int
		wantView       hotstuff.View
	}{
		// four signers; quorum reached, advance view
		{name: "signers=4/Simple___/__/__/__", tr: S, qc: F, tc: F, ac: F, firstSignerIdx: 0, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=4/Simple___/__/__/AC", tr: S, qc: F, tc: F, ac: T, firstSignerIdx: 0, wantView: 1}, // simple timeout rule ignores aggregate timeout cert, will not advance view
		{name: "signers=4/Simple___/__/TC/__", tr: S, qc: F, tc: T, ac: F, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Simple___/__/TC/AC", tr: S, qc: F, tc: T, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Simple___/QC/__/__", tr: S, qc: T, tc: F, ac: F, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Simple___/QC/__/AC", tr: S, qc: T, tc: F, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Simple___/QC/TC/AC", tr: S, qc: T, tc: T, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Aggregate/__/__/__", tr: A, qc: F, tc: F, ac: F, firstSignerIdx: 0, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=4/Aggregate/__/__/AC", tr: A, qc: F, tc: F, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Aggregate/__/TC/__", tr: A, qc: F, tc: T, ac: F, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Aggregate/__/TC/AC", tr: A, qc: F, tc: T, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Aggregate/QC/__/__", tr: A, qc: T, tc: F, ac: F, firstSignerIdx: 0, wantView: 1}, // aggregate timeout rule ignores quorum cert, will not advance view
		{name: "signers=4/Aggregate/QC/__/AC", tr: A, qc: T, tc: F, ac: T, firstSignerIdx: 0, wantView: 2},
		{name: "signers=4/Aggregate/QC/TC/AC", tr: A, qc: T, tc: T, ac: T, firstSignerIdx: 0, wantView: 2},
		// three signers; quorum reacted, advance view
		{name: "signers=3/Simple___/__/__/__", tr: S, qc: F, tc: F, ac: F, firstSignerIdx: 1, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=3/Simple___/__/__/AC", tr: S, qc: F, tc: F, ac: T, firstSignerIdx: 1, wantView: 1}, // simple timeout rule ignores aggregate timeout cert, will not advance view
		{name: "signers=3/Simple___/__/TC/__", tr: S, qc: F, tc: T, ac: F, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Simple___/__/TC/AC", tr: S, qc: F, tc: T, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Simple___/QC/__/__", tr: S, qc: T, tc: F, ac: F, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Simple___/QC/__/AC", tr: S, qc: T, tc: F, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Simple___/QC/TC/AC", tr: S, qc: T, tc: T, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Aggregate/__/__/__", tr: A, qc: F, tc: F, ac: F, firstSignerIdx: 1, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=3/Aggregate/__/__/AC", tr: A, qc: F, tc: F, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Aggregate/__/TC/__", tr: A, qc: F, tc: T, ac: F, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Aggregate/__/TC/AC", tr: A, qc: F, tc: T, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Aggregate/QC/__/__", tr: A, qc: T, tc: F, ac: F, firstSignerIdx: 1, wantView: 1}, // aggregate timeout rule ignores quorum cert, will not advance view
		{name: "signers=3/Aggregate/QC/__/AC", tr: A, qc: T, tc: F, ac: T, firstSignerIdx: 1, wantView: 2},
		{name: "signers=3/Aggregate/QC/TC/AC", tr: A, qc: T, tc: T, ac: T, firstSignerIdx: 1, wantView: 2},
		// only two signers; no quorum reached, should not advance view
		{name: "signers=2/Simple___/__/__/__", tr: S, qc: F, tc: F, ac: F, firstSignerIdx: 2, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=2/Simple___/__/__/AC", tr: S, qc: F, tc: F, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Simple___/__/TC/__", tr: S, qc: F, tc: T, ac: F, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Simple___/__/TC/AC", tr: S, qc: F, tc: T, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Simple___/QC/__/__", tr: S, qc: T, tc: F, ac: F, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Simple___/QC/__/AC", tr: S, qc: T, tc: F, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Simple___/QC/TC/AC", tr: S, qc: T, tc: T, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/__/__/__", tr: A, qc: F, tc: F, ac: F, firstSignerIdx: 2, wantView: 1}, // empty syncInfo, should not advance view
		{name: "signers=2/Aggregate/__/__/AC", tr: A, qc: F, tc: F, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/__/TC/__", tr: A, qc: F, tc: T, ac: F, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/__/TC/AC", tr: A, qc: F, tc: T, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/QC/__/__", tr: A, qc: T, tc: F, ac: F, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/QC/__/AC", tr: A, qc: T, tc: F, ac: T, firstSignerIdx: 2, wantView: 1},
		{name: "signers=2/Aggregate/QC/TC/AC", tr: A, qc: T, tc: T, ac: T, firstSignerIdx: 2, wantView: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, viewStates, synchronizer, block := prepareSynchronizer(t)
			signers := set.Signers()
			essentials := set[0]
			// this is a workaround since we want to test both timeout rules in the same test
			if tt.tr == A {
				synchronizer.timeoutRules = NewAggregate(essentials.RuntimeCfg(), essentials.Authority())
			} else {
				synchronizer.timeoutRules = NewSimple(essentials.RuntimeCfg(), essentials.Authority())
			}

			syncInfo := hotstuff.NewSyncInfo()
			if tt.qc {
				validQC := testutil.CreateQC(t, block, signers[tt.firstSignerIdx:]...)
				syncInfo = syncInfo.WithQC(validQC)
			}
			if tt.tc {
				validTC := testutil.CreateTC(t, 1, signers[tt.firstSignerIdx:])
				syncInfo = syncInfo.WithTC(validTC)
			}
			if tt.ac {
				validAC := testutil.CreateAC(t, 1, signers[tt.firstSignerIdx:])
				syncInfo = syncInfo.WithAggQC(validAC)
			}

			// t.Logf("  %s: SyncInfo: %v", tt.name, syncInfo)
			// t.Logf("B %s: HighQC.View: %d, HighTC.View: %d", tt.name, viewStates.HighQC().View(), viewStates.HighTC().View())
			synchronizer.advanceView(syncInfo)
			if viewStates.View() != tt.wantView {
				t.Errorf("View() = %d, want %d", viewStates.View(), tt.wantView)
			}
			// t.Logf("A %s: HighQC.View: %d, HighTC.View: %d", tt.name, viewStates.HighQC().View(), viewStates.HighTC().View())
		})
	}
}

func prepareSynchronizer(t *testing.T) (testutil.EssentialsSet, *protocol.ViewStates, *Synchronizer, *hotstuff.Block) {
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
	block := testutil.CreateBlock(t, subject.Authority())
	blockchain.Store(block)

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
	return set, viewStates, synchronizer, block
}
