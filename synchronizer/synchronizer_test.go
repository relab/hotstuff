package synchronizer

import (
	"bytes"
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
	s := New(100 * time.Millisecond)
	builder.Register(hs, s)
	mods := builder.Build()
	cfg := mods.Config().(*mocks.MockConfig)

	c := make(chan struct{})
	hs.EXPECT().IncreaseLastVotedView(hotstuff.View(1))
	hs.EXPECT().HighQC().Return(qc)
	cfg.
		EXPECT().
		Timeout(gomock.AssignableToTypeOf(&hotstuff.TimeoutMsg{})).
		Do(func(msg *hotstuff.TimeoutMsg) {
			if msg.View != 1 {
				t.Errorf("wrong view. got: %v, want: %v", msg.View, 1)
			}
			if msg.ID != 2 {
				t.Errorf("wrong ID. got: %v, want: %v", msg.ID, 1)
			}
			if !bytes.Equal(msg.HighQC.ToBytes(), qc.ToBytes()) {
				t.Errorf("wrong QC. got: %v, want: %v", msg.HighQC, qc)
			}
			if !mods.Verifier().Verify(msg.Signature, msg.View.ToHash()) {
				t.Error("failed to verify signature")
			}
			close(c)
		})

	s.Start()
	<-c
}
