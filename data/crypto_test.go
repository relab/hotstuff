package data

import (
	"crypto/ecdsa"
	"testing"
)

var pk ecdsa.PrivateKey // must not be a pointer

var simpleRc = &ReplicaConfig{
	Replicas: map[ReplicaID]*ReplicaInfo{
		0: {
			ID:      0,
			Address: "",
			PubKey:  &pk.PublicKey, // this is why
		},
	},
	QuorumSize: 1,
}

var biggerRc = &ReplicaConfig{
	Replicas: map[ReplicaID]*ReplicaInfo{
		0: {
			ID:      0,
			Address: "",
			PubKey:  &pk.PublicKey,
		},
		1: {
			ID:      1,
			Address: "",
			PubKey:  &pk.PublicKey,
		},
		2: {
			ID:      2,
			Address: "",
			PubKey:  &pk.PublicKey,
		},
		3: {
			ID:      3,
			Address: "",
			PubKey:  &pk.PublicKey,
		},
	},
	QuorumSize: 3,
}

var testBlock = &Block{
	Commands: []Command{Command("this is a test")},
	Height:   0,
}

func init() {
	k, err := GeneratePrivateKey()
	if err != nil {
		panic(err)
	}
	pk = *k
}

func createPartialCert(t *testing.T, id ReplicaID) *PartialCert {
	pc, err := CreatePartialCert(id, &pk, testBlock)
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

func createQuorumCert(t *testing.T) *QuorumCert {
	qc := CreateQuorumCert(testBlock)
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

func BenchmarkQuroumCertToBytes(b *testing.B) {
	qc := CreateQuorumCert(testBlock)
	for _, r := range biggerRc.Replicas {
		pc, _ := CreatePartialCert(r.ID, &pk, testBlock)
		qc.AddPartial(pc)
	}
	for n := 0; n < b.N; n++ {
		qc.toBytes()
	}
}

func BenchmarkPartialSigToBytes(b *testing.B) {
	pc, _ := CreatePartialCert(0, &pk, testBlock)
	for n := 0; n < b.N; n++ {
		pc.Sig.toBytes()
	}
}
