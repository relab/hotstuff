package synchronizer_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	. "github.com/relab/hotstuff/synchronizer"
)

func TestLocalTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	qc := consensus.NewQuorumCert(nil, 0, consensus.GetGenesis().Hash())
	builder := testutil.TestModules(t, ctrl, 2, testutil.GenerateECDSAKey(t))
	hs := mocks.NewMockConsensus(ctrl)
	s := New(testutil.FixedTimeout(10))
	builder.Register(hs, s)
	mods := builder.Build()
	cfg := mods.Configuration().(*mocks.MockConfiguration)
	leader := testutil.CreateMockReplica(t, ctrl, 1, testutil.GenerateECDSAKey(t))
	testutil.ConfigAddReplica(t, cfg, leader)

	c := make(chan struct{})
	hs.EXPECT().StopVoting(consensus.View(1)).AnyTimes()
	cfg.
		EXPECT().
		Timeout(gomock.AssignableToTypeOf(consensus.TimeoutMsg{})).
		Do(func(msg consensus.TimeoutMsg) {
			if msg.View != 1 {
				t.Errorf("wrong view. got: %v, want: %v", msg.View, 1)
			}
			if msg.ID != 2 {
				t.Errorf("wrong ID. got: %v, want: %v", msg.ID, 2)
			}
			if msgQC, ok := msg.SyncInfo.QC(); ok && !bytes.Equal(msgQC.ToBytes(), qc.ToBytes()) {
				t.Errorf("wrong QC. got: %v, want: %v", msgQC, qc)
			}
			if !mods.Crypto().Verify(msg.ViewSignature, msg.View.ToHash()) {
				t.Error("failed to verify signature")
			}
			c <- struct{}{}
		}).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		mods.Synchronizer().Start(ctx)
		mods.Run(ctx)
	}()
	<-c
	cancel()
}

func TestAdvanceViewQC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := New(testutil.FixedTimeout(1000))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Register(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	block := consensus.NewBlock(
		consensus.GetGenesis().Hash(),
		consensus.NewQuorumCert(nil, 0, consensus.GetGenesis().Hash()),
		"foo",
		1,
		2,
	)
	hl[0].BlockChain().Store(block)
	qc := testutil.CreateQC(t, block, signers)
	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(consensus.NewSyncInfo()))

	s.AdvanceView(consensus.NewSyncInfo().WithQC(qc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := New(testutil.FixedTimeout(100))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Register(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	tc := testutil.CreateTC(t, 1, signers)

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(gomock.AssignableToTypeOf(consensus.NewSyncInfo()))

	s.AdvanceView(consensus.NewSyncInfo().WithTC(tc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

// func TestRemoteTimeout(t *testing.T) {
// 	const n = 4
// 	ctrl := gomock.NewController(t)
// 	builders := testutil.CreateBuilders(t, ctrl, n)
// 	s := New(testutil.FixedTimeout(100))
// 	hs := mocks.NewMockConsensus(ctrl)
// 	builders[0].Register(s, hs)

// 	hl := builders.Build()
// 	signers := hl.Signers()

// 	timeouts := testutil.CreateTimeouts(t, 1, signers[1:])

// 	// synchronizer should tell hotstuff to propose
// 	hs.EXPECT().Propose(gomock.AssignableToTypeOf(consensus.NewSyncInfo()))

// 	for _, timeout := range timeouts {
// 		s.OnRemoteTimeout(timeout)
// 	}

// 	if s.View() != 2 {
// 		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
// 	}
// }
