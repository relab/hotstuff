package proto

import (
	bytes "bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	math "math"
	"math/big"
	"testing"

	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/data"
)

var pk ecdsa.PrivateKey

func init() {
	_pk, _ := data.GeneratePrivateKey()
	pk = *_pk
}

var simpleRc = config.ReplicaConfig{
	Replicas: map[config.ReplicaID]*config.ReplicaInfo{
		0: {
			ID:      0,
			Address: "",
			PubKey:  &pk.PublicKey, // this is why
		},
	},
	QuorumSize: 1,
}

var testBlock = data.Block{
	Commands: []data.Command{data.Command("this is a test")},
	Height:   0,
}

func TestMarshalingPartialCertToProto(t *testing.T) {
	pc1, _ := data.CreatePartialCert(config.ReplicaID(0), &pk, &testBlock)

	ppc := PartialCertToProto(pc1)
	pc2 := ppc.FromProto()

	if !bytes.Equal(pc1.BlockHash[:], pc2.BlockHash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(pc2.BlockHash[:]), hex.EncodeToString(pc1.BlockHash[:]))
	}

	if !data.VerifyPartialCert(&simpleRc, pc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}

func TestMarshalingQuorumCertToProto(t *testing.T) {
	qc1 := data.CreateQuorumCert(&testBlock)
	pc1, _ := data.CreatePartialCert(0, &pk, &testBlock)
	qc1.AddPartial(pc1)
	pqc := QuorumCertToProto(qc1)
	qc2 := pqc.FromProto()

	if !bytes.Equal(qc1.BlockHash[:], qc2.BlockHash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(qc2.BlockHash[:]), hex.EncodeToString(qc1.BlockHash[:]))
	}

	if !data.VerifyQuorumCert(&simpleRc, qc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}

func TestMarshalAndUnmarshalBlock(t *testing.T) {
	testBlock := &data.Block{Commands: []data.Command{data.Command("test")}}
	testQC := data.CreateQuorumCert(testBlock)
	numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
	for j := int64(0); j < numSigs.Int64(); j++ {
		id, _ := rand.Int(rand.Reader, big.NewInt(1000))
		r, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		s, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		sig := &data.PartialSig{ID: config.ReplicaID(id.Int64()), R: r, S: s}
		cert := &data.PartialCert{Sig: *sig, BlockHash: testBlock.Hash()}
		testQC.AddPartial(cert)
	}

	testBlock.Justify = testQC

	h1 := testBlock.Hash()
	protoBlock := BlockToProto(testBlock)
	testBlock2 := protoBlock.FromProto()
	h2 := testBlock2.Hash()

	if !bytes.Equal(h1[:], h2[:]) {
		t.Fatalf("Hashes don't match after marshaling / unmarshaling!")
	}
}
