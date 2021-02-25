package proto

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestConvertPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pk := testutil.GenerateKey(t)

	cfg := testutil.CreateMockConfig(t, ctrl, 1, pk)
	signer, _ := ecdsa.New(cfg)
	want, err := signer.CreatePartialCert(hotstuff.GetGenesis())
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), nil, "", 1, 1)
	getSignature := func(id hotstuff.ID) hotstuff.PartialCert {
		key := testutil.GenerateKey(t)
		cfg := testutil.CreateMockConfig(t, ctrl, id, key)
		signer, _ := ecdsa.New(cfg)
		pcert, err := signer.CreatePartialCert(b1)
		if err != nil {
			t.Fatal(err)
		}
		return pcert
	}

	sig1 := getSignature(1)
	sig2 := getSignature(2)

	key := testutil.GenerateKey(t)
	cfg := testutil.CreateMockConfig(t, ctrl, 0, key)
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
	qc := ecdsa.NewQuorumCert(map[hotstuff.ID]*ecdsa.Signature{}, hotstuff.Hash{})
	want := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "", 1, 1)
	pb := BlockToProto(want)
	got := BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}
