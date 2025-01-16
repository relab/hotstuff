package synchronizer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
	"go.uber.org/mock/gomock"
)

func TestAdvanceViewQC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := synchronizer.New(testutil.FixedTimeout(1000))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Add(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()),
		"foo",
		1,
		2,
	)

	var blockChain modules.BlockChain
	hl[0].Get(&blockChain)

	blockChain.Store(block)
	qc := testutil.CreateQC(t, block, signers)
	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo()))

	s.AdvanceView(hotstuff.NewSyncInfo().WithQC(qc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := synchronizer.New(testutil.FixedTimeout(100))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Add(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	tc := testutil.CreateTC(t, 1, signers)

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo()))

	s.AdvanceView(hotstuff.NewSyncInfo().WithTC(tc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestViewDurationViewStarted(t *testing.T) {
	startDuration := 500 * time.Millisecond
	maxDuration := 1000 * time.Second
	vd := synchronizer.NewViewDuration(5, 2, startDuration, maxDuration)

	vd.ViewStarted()
	d := vd.Duration()
	if d != startDuration {
		t.Errorf("call 1: expected view duration to stay the same. want: %v, got: %v", startDuration, d)
	}

	vd.ViewStarted()
	d = vd.Duration()
	fmt.Printf("%v\n", d)
	if d != startDuration {
		t.Errorf("call 2: expected view duration to stay the same. want: %v, got: %v", startDuration, d)
	}
}

func TestViewDurationViewTimeout(t *testing.T) {
	startDuration := 500 * time.Millisecond
	maxDuration := 2000 * time.Second
	mult := float32(2.0)

	vd := synchronizer.NewViewDuration(5, mult, startDuration, maxDuration)

	vd.ViewStarted()
	vd.ViewTimeout()

	d := vd.Duration()
	want := startDuration * time.Duration(mult)
	if d != want {
		t.Errorf("expected duration to be %f times larger. want: %v, go %v", mult, want, d)
	}
}

func TestViewDurationMax(t *testing.T) {
	startDuration := 100 * time.Millisecond
	maxDuration := 500 * time.Millisecond
	mult := float32(5.0)

	vd := synchronizer.NewViewDuration(5, mult, startDuration, maxDuration)

	vd.ViewStarted()
	vd.ViewTimeout()

	d := vd.Duration()
	want := maxDuration
	if d != want {
		t.Errorf("expected duration to be the max. want: %v, go %v", want, d)
	}
}
