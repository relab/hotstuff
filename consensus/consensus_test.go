package consensus_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	bl := testutil.CreateBuilders(t, ctrl, n)
	cs := mocks.NewMockConsensus(ctrl)
	d := testutil.FixedTimeout(1 * time.Millisecond)
	bl[0].Add(synchronizer.New(), d, cs)
	hl := bl.Build()
	hs := hl[0]

	var (
		eventLoop  *eventloop.ScopedEventLoop
		blockChain modules.BlockChain
	)

	hs.Get(&eventLoop, &blockChain)

	cs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo(hotstuff.NullPipe)))

	ok := false
	ctx, cancel := context.WithCancel(context.Background())
	eventLoop.RegisterObserver(hotstuff.NewViewMsg{}, func(_ any) {
		ok = true
		cancel()
	})

	b := testutil.NewProposeMsg(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(
			nil,
			1,
			hotstuff.NullPipe, // TODO: Verify if this code conflicts with pipelining
			hotstuff.GetGenesis().Hash()),
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
