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

	builder := testutil.TestModules(t, ctrl, 1, testutil.GenerateKey(t))
	hs := builder.Build()
	signer := hs.Signer()

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

	builders := testutil.CreateBuilders(t, ctrl, 4)
	hl := builders.Build()

	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), nil, "", 1, 1)

	signatures := testutil.CreatePCs(t, b1, hl.Signers())

	want, err := hl[0].Signer().CreateQuorumCert(b1, signatures)
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
