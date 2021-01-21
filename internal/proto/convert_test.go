package proto

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
)

func getMockConfig(t *testing.T, id hotstuff.ID) *mocks.MockConfig {
	t.Helper()
	pk, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	ctrl := gomock.NewController(t)
	cfg := mocks.NewMockConfig(ctrl)
	cfg.
		EXPECT().
		PrivateKey().
		AnyTimes().
		Return(&ecdsa.PrivateKey{PrivateKey: pk})
	cfg.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)
	return cfg
}

func TestConvertPartialCert(t *testing.T) {
	cfg := getMockConfig(t, 1)
	signer, _ := ecdsa.New(cfg)
	want, err := signer.Sign(hotstuff.GetGenesis())
	if err != nil {
		t.Fatal(err)
	}

	pb := PartialCertToProto(want)
	got := PartialCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertQuorumCert(t *testing.T) {
	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), nil, "", 1, 1)
	getSignature := func(id hotstuff.ID) hotstuff.PartialCert {
		cfg := getMockConfig(t, id)
		signer, _ := ecdsa.New(cfg)
		pcert, err := signer.Sign(b1)
		if err != nil {
			t.Fatal(err)
		}
		return pcert
	}

	sig1 := getSignature(1)
	sig2 := getSignature(2)

	cfg := getMockConfig(t, 0)
	signer, _ := ecdsa.New(cfg)

	want, err := signer.CreateQuorumCert(b1, []hotstuff.PartialCert{sig1, sig2})
	if err != nil {
		t.Fatal(err)
	}

	pb := QuorumCertToProto(want)
	got := QuorumCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertBlock(t *testing.T) {
	qc := ecdsa.NewQuorumCert(map[hotstuff.ID]ecdsa.Signature{}, hotstuff.Hash{})
	want := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), &qc, "", 1, 1)
	pb := BlockToProto(want)
	got := BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}
