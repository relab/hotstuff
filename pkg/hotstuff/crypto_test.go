package hotstuff

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"testing"
)

var pk ecdsa.PrivateKey // must not be a pointer

var simpleRc = &ReplicaConfig{
	Replicas: map[ReplicaID]*ReplicaInfo{
		0: {
			ID:     0,
			Socket: "",
			PubKey: &pk.PublicKey, // this is why
		},
	},
	QuorumSize: 1,
}

var biggerRc = &ReplicaConfig{
	Replicas: map[ReplicaID]*ReplicaInfo{
		0: {
			ID:     0,
			Socket: "",
			PubKey: &pk.PublicKey,
		},
		1: {
			ID:     1,
			Socket: "",
			PubKey: &pk.PublicKey,
		},
		2: {
			ID:     2,
			Socket: "",
			PubKey: &pk.PublicKey,
		},
		3: {
			ID:     3,
			Socket: "",
			PubKey: &pk.PublicKey,
		},
	},
	QuorumSize: 3,
}

var testNode = &Node{
	Command: []byte("this is a test"),
	Height:  0,
}

func init() {
	k, err := GeneratePrivateKey()
	if err != nil {
		panic(err)
	}
	pk = *k
}

func createPartialCert(t *testing.T, id ReplicaID) *PartialCert {
	pc, err := CreatePartialCert(id, &pk, testNode)
	if err != nil {
		t.Errorf("Failed to create partial certificate: %v\n", err)
	}
	return pc
}

func TestVerifyPartialCert(t *testing.T) {
	pc := createPartialCert(t, 0)

	if !VerifyPartialCert(simpleRc, pc) {
		t.Errorf("Partial cert failed to verify!")
	}
}

func TestMarshalingPartialCertToProto(t *testing.T) {
	pc1 := createPartialCert(t, 0)

	ppc := pc1.toProto()
	pc2 := partialCertFromProto(ppc)

	if !bytes.Equal(pc1.hash[:], pc2.hash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(pc2.hash[:]), hex.EncodeToString(pc1.hash[:]))
	}

	if !VerifyPartialCert(simpleRc, pc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}

func createQuorumCert(t *testing.T) *QuorumCert {
	qc := CreateQuorumCert(testNode)
	for k := range biggerRc.Replicas {
		err := qc.AddPartial(createPartialCert(t, k))
		if err != nil {
			t.Errorf("Failed to add partial cert to quorum cert: %v\n", err)
		}
	}
	return qc
}

func TestVerifyQuorumCert(t *testing.T) {
	qc := createQuorumCert(t)
	if !VerifyQuorumCert(biggerRc, qc) {
		t.Errorf("Quorum cert failed to verify!")
	}
}

func TestMarshalingQuorumCertToProto(t *testing.T) {
	qc1 := createQuorumCert(t)
	pqc := qc1.toProto()
	qc2 := quorumCertFromProto(pqc)

	if !bytes.Equal(qc1.hash[:], qc2.hash[:]) {
		t.Errorf("Hashes don't match! Got %v, want: %v\n",
			hex.EncodeToString(qc2.hash[:]), hex.EncodeToString(qc1.hash[:]))
	}

	if !VerifyQuorumCert(biggerRc, qc2) {
		t.Errorf("Cert failed to verify!\n")
	}
}
