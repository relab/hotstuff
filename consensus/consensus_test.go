package consensus_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/synchronizer"
	"go.uber.org/mock/gomock"
)

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	bl := testutil.CreateBuilders(t, ctrl, n)
	cs := mocks.NewMockConsensus(ctrl)
	bl[0].Add(testutil.FixedTimeout(1*time.Millisecond), synchronizer.New(), cs)
	hl := bl.Build()
	hs := hl[0]

	var (
		eventLoop  *core.EventLoop
		blockChain core.BlockChain
	)

	hs.Get(&eventLoop, &blockChain)

	cs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo()))

	ok := false
	ctx, cancel := context.WithCancel(context.Background())
	eventLoop.RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		ok = true
		cancel()
	}, core.Prioritize())

	b := testutil.NewProposeMsg(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 1, hotstuff.GetGenesis().Hash()),
		"test", 1, 1,
	)
	blockChain.Store(b.Block)

	for i, signer := range hl.Signers() {
		pc, err := signer.CreatePartialCert(b.Block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}
		eventLoop.AddEvent(hotstuff.VoteMsg{ID: hotstuff.ID(i + 1), PartialCert: pc})
	}

	eventLoop.Run(ctx)

	if !ok {
		t.Error("No new view event happened")
	}
}
