package hotstuffpb

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestConvertPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)

	builder := testutil.TestModules(t, ctrl, 1, testutil.GenerateECDSAKey(t))
	hs := builder.Build()
	signer := hs.Crypto()

	want, err := signer.CreatePartialCert(consensus.GetGenesis())
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

	b1 := consensus.NewBlock(consensus.GetGenesis().Hash(), consensus.NewQuorumCert(nil, 0, consensus.GetGenesis().Hash()), "", 1, 1)

	signatures := testutil.CreatePCs(t, b1, hl.Signers())

	want, err := hl[0].Crypto().CreateQuorumCert(b1, signatures)
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
	qc := consensus.NewQuorumCert(nil, 0, consensus.Hash{})
	want := consensus.NewBlock(consensus.GetGenesis().Hash(), qc, "", 1, 1)
	pb := BlockToProto(want)
	got := BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}

func TestConvertTimeoutCertBLS12(t *testing.T) {
	ctrl := gomock.NewController(t)

	builders := testutil.CreateBuilders(t, ctrl, 4, testutil.GenerateKeys(t, 4, testutil.GenerateBLS12Key)...)
	for i := range builders {
		builders[i].Register(crypto.New(bls12.New()))
	}
	hl := builders.Build()

	tc1 := testutil.CreateTC(t, 1, hl.Signers())

	pb := TimeoutCertToProto(tc1)
	tc2 := TimeoutCertFromProto(pb)

	if !hl[0].Crypto().VerifyTimeoutCert(tc2) {
		t.Fatal("Failed to verify timeout cert")
	}
}
