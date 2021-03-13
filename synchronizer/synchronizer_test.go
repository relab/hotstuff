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
	"github.com/relab/hotstuff/leaderrotation"
)

func TestLocalTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	qc := ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), hotstuff.GetGenesis().Hash())
	cfg := testutil.CreateMockConfig(t, ctrl, 1, testutil.GenerateKey(t))
	replica := testutil.CreateMockReplica(t, ctrl, 1, cfg.PrivateKey().Public())
	testutil.ConfigAddReplica(t, cfg, replica)
	signer, verifier := ecdsa.New(cfg)
	hs := mocks.NewMockConsensus(ctrl)

	hs.EXPECT().Config().AnyTimes().Return(cfg)
	hs.EXPECT().HighQC().AnyTimes().Return(qc)
	hs.EXPECT().Signer().AnyTimes().Return(signer)
	hs.EXPECT().Verifier().AnyTimes().Return(verifier)
	hs.EXPECT().IncreaseLastVotedView(hotstuff.View(1))

	c := make(chan struct{})
	cfg.
		EXPECT().
		Timeout(gomock.AssignableToTypeOf(&hotstuff.TimeoutMsg{})).
		Do(func(msg *hotstuff.TimeoutMsg) {
			if msg.View != 1 {
				t.Errorf("wrong view. got: %v, want: %v", msg.View, 1)
			}
			if msg.ID != 1 {
				t.Errorf("wrong ID. got: %v, want: %v", msg.ID, 1)
			}
			if !bytes.Equal(msg.HighQC.ToBytes(), qc.ToBytes()) {
				t.Errorf("wrong QC. got: %v, want: %v", msg.HighQC, qc)
			}
			if !verifier.Verify(msg.Signature, msg.View.ToHash()) {
				t.Error("failed to verify signature")
			}
			close(c)
		})

	s := New(leaderrotation.NewFixed(2), 100*time.Millisecond)
	s.Init(hs)
	s.Start()
	<-c
}

/* func TestRemoteTimeouts(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	keys := make([]hotstuff.PrivateKey, n)
	replicas := make([]*mocks.MockReplica, n)
	configs := make([]*mocks.MockConfig, n)
	signers := make([]hotstuff.signer, n)
	verifiers := make([]hotstuff.verifier, n)
	for i := 0; i < n; i++ {
		id := hotstuff.ID(i + 1)
		keys[i] = testutil.GenerateKey(t)
		replicas[i] = testutil.CreateMockReplica(t, ctrl, id, keys[i].Public())
		configs[i] = testutil.CreateMockConfig(t, ctrl, id, keys[i])
		signers[i], verifiers[i] = ecdsa.New(configs[i])
	}
	for _, config := range configs {
		for _, replica := range replicas {
			testutil.ConfigAddReplica(t, config, replica)
		}
	}

	signatures := testutil.CreateSignatures(t, hotstuff.View(1).ToHash(), signers)

	s := New(leaderrotation.NewFixed(2), 100*time.Millisecond)

	s.OnRemoteTimeout()
} */
