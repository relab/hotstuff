package synchronizer

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestLocalTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	qc := ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), hotstuff.GetGenesis().Hash())
	builder := testutil.TestModules(t, ctrl, 2, testutil.GenerateKey(t))
	hs := mocks.NewMockConsensus(ctrl)
	s := New(testutil.FixedTimeout(time.Millisecond))
	builder.Register(hs, s)
	mods := builder.Build()
	cfg := mods.Config().(*mocks.MockConfig)
	leader := testutil.CreateMockReplica(t, ctrl, 1, testutil.GenerateKey(t))
	testutil.ConfigAddReplica(t, cfg, leader)

	c := make(chan struct{})
	hs.EXPECT().IncreaseLastVotedView(hotstuff.View(1)).AnyTimes()
	cfg.
		EXPECT().
		Timeout(gomock.AssignableToTypeOf(hotstuff.TimeoutMsg{})).
		Do(func(msg hotstuff.TimeoutMsg) {
			if msg.View != 1 {
				t.Errorf("wrong view. got: %v, want: %v", msg.View, 1)
			}
			if msg.ID != 2 {
				t.Errorf("wrong ID. got: %v, want: %v", msg.ID, 2)
			}
			if !bytes.Equal(msg.SyncInfo.QC.ToBytes(), qc.ToBytes()) {
				t.Errorf("wrong QC. got: %v, want: %v", msg.SyncInfo.QC, qc)
			}
			if !mods.Verifier().Verify(msg.Signature, msg.View.ToHash()) {
				t.Error("failed to verify signature")
			}
			close(c)
		})
	ctx, cancel := context.WithCancel(context.Background())
	go mods.EventLoop().Run(ctx)
	<-c
	cancel()
}

func TestAdvanceViewQC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := New(testutil.FixedTimeout(100 * time.Millisecond))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Register(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), hotstuff.GetGenesis().Hash()),
		"foo",
		1,
		2,
	)
	hl[0].BlockChain().Store(block)
	qc := testutil.CreateQC(t, block, signers)
	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose()

	s.AdvanceView(hotstuff.SyncInfo{QC: qc})

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := New(testutil.FixedTimeout(100 * time.Millisecond))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Register(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	tc := testutil.CreateTC(t, 1, signers)

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose()

	s.AdvanceView(hotstuff.SyncInfo{TC: tc})

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}

func TestRemoteTimeout(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := New(testutil.FixedTimeout(100 * time.Millisecond))
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Register(s, hs)

	hl := builders.Build()
	signers := hl.Signers()

	timeouts := testutil.CreateTimeouts(t, 1, signers[1:])

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose()

	for _, timeout := range timeouts {
		s.OnRemoteTimeout(timeout)
	}

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}
