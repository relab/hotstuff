package hotstuff

import (
	"bytes"
	"crypto/rand"
	"math"
	"math/big"
	"testing"

	fuzz "github.com/google/gofuzz"
)

func TestCanStoreAndRetrieveNode(t *testing.T) {
	nodes := NewMapStorage()
	testNode := &Node{Command: []byte("Hello world")}

	nodes.Put(testNode)
	got, ok := nodes.Get(testNode.Hash())

	if !ok || !bytes.Equal(testNode.Command, got.Command) {
		t.Errorf("Failed to retrieve node from storage.")
	}
}

// Checks that the hash function for nodes is deterministic
func TestFuzzNodeHash(t *testing.T) {
	f := fuzz.New()
	for i := 0; i < 10000; i++ {
		var testNode Node
		f.Fuzz(&testNode)
		testNode.Justify = CreateQuorumCert(&testNode)
		numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
		for j := int64(0); j < numSigs.Int64(); j++ {
			var sig partialSig
			f.Fuzz(&sig)
			id, _ := rand.Int(rand.Reader, big.NewInt(1000))
			rID := ReplicaID(id.Int64())
			sig.id = rID
			sig.r, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			sig.s, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			testNode.Justify.sigs[rID] = sig
		}
		hash1 := testNode.Hash()
		hash2 := testNode.Hash()
		if !bytes.Equal(hash1[:], hash2[:]) {
			t.Fatalf("Non-determinism in hash function detected:\nNode: %s\nHash1: %s\nHash2: %s", testNode, hash1, hash2)
		}
	}
}

func TestMarshalAndUnmarshalNode(t *testing.T) {
	testNode := &Node{Command: []byte("test")}

	testQC := CreateQuorumCert(testNode)
	numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
	for j := int64(0); j < numSigs.Int64(); j++ {
		var sig partialSig
		id, _ := rand.Int(rand.Reader, big.NewInt(1000))
		sig.id = ReplicaID(id.Int64())
		sig.r, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		sig.s, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		testQC.sigs[sig.id] = sig
	}

	testNode.Justify = testQC

	h1 := testNode.Hash()
	protoNode := testNode.toProto()
	testNode2 := nodeFromProto(protoNode)
	h2 := testNode2.Hash()

	if !bytes.Equal(h1[:], h2[:]) {
		t.Fatalf("Hashes don't match after marshaling / unmarshaling!")
	}
}
