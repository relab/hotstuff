package proto

import (
	bytes "bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	math "math"
	"math/big"
	"testing"

	"github.com/relab/hotstuff"
)

var pk ecdsa.PrivateKey

func init() {
	_pk, _ := hotstuff.GeneratePrivateKey()
	pk = *_pk
}

var simpleRc = hotstuff.ReplicaConfig{
	Replicas: map[hotstuff.ReplicaID]*hotstuff.ReplicaInfo{
		0: {
			ID:      0,
			Address: "",
			PubKey:  &pk.PublicKey, // this is why
		},
	},
	QuorumSize: 1,
}

var testNode = hotstuff.Node{
	Command: []byte("this is a test"),
	Height:  0,
}

func TestMarshalingPartialCertToProto(t *testing.T) {
	pc1, _ := hotstuff.CreatePartialCert(hotstuff.ReplicaID(0), &pk, &testNode)

	ppc := PartialCertToProto(pc1)
	pc2 := ppc.FromProto()

	if !bytes.Equal(pc1.NodeHash[:], pc2.NodeHash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(pc2.NodeHash[:]), hex.EncodeToString(pc1.NodeHash[:]))
	}

	if !hotstuff.VerifyPartialCert(&simpleRc, pc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}

func TestMarshalingQuorumCertToProto(t *testing.T) {
	qc1 := hotstuff.CreateQuorumCert(&testNode)
	pc1, _ := hotstuff.CreatePartialCert(0, &pk, &testNode)
	qc1.AddPartial(pc1)
	pqc := QuorumCertToProto(qc1)
	qc2 := pqc.FromProto()

	if !bytes.Equal(qc1.NodeHash[:], qc2.NodeHash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(qc2.NodeHash[:]), hex.EncodeToString(qc1.NodeHash[:]))
	}

	if !hotstuff.VerifyQuorumCert(&simpleRc, qc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}

func TestMarshalAndUnmarshalNode(t *testing.T) {
	testNode := &hotstuff.Node{Command: []byte("test")}
	testQC := hotstuff.CreateQuorumCert(testNode)
	numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
	for j := int64(0); j < numSigs.Int64(); j++ {
		id, _ := rand.Int(rand.Reader, big.NewInt(1000))
		r, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		s, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		sig := &hotstuff.PartialSig{ID: hotstuff.ReplicaID(id.Int64()), R: r, S: s}
		cert := &hotstuff.PartialCert{Sig: *sig, NodeHash: testNode.Hash()}
		testQC.AddPartial(cert)
	}

	testNode.Justify = testQC

	h1 := testNode.Hash()
	protoNode := NodeToProto(testNode)
	testNode2 := protoNode.FromProto()
	h2 := testNode2.Hash()

	if !bytes.Equal(h1[:], h2[:]) {
		t.Fatalf("Hashes don't match after marshaling / unmarshaling!")
	}
}
