package synchronizer_test

import (
	"testing"

	"github.com/relab/hotstuff"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

func TestAdvanceViewQC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := synchronizer.New()
	d := testutil.FixedTimeout(1000)
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Add(s, d, hs)

	hl := builders.Build()
	signers := hl.Signers()

	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(
			nil,
			0,
			hotstuff.NullPipe, // TODO: Verify if this code conflicts with pipelining
			hotstuff.GetGenesis().Hash()),
		"foo",
		1,
		2,
		0,
	)

	var blockChain modules.BlockChain
	hl[0].Get(&blockChain)

	blockChain.Store(block)
	qc := testutil.CreateQC(t, block, signers)
	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo(hotstuff.NullPipe)))

	s.AdvanceView(hotstuff.NewSyncInfo(hotstuff.NullPipe).WithQC(qc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := synchronizer.New()
	d := testutil.FixedTimeout(100)
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Add(s, d, hs)

	hl := builders.Build()
	signers := hl.Signers()

	tc := testutil.CreateTC(t, 1, signers)

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.NewSyncInfo(hotstuff.NullPipe)))

	s.AdvanceView(hotstuff.NewSyncInfo(hotstuff.NullPipe).WithTC(tc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}
