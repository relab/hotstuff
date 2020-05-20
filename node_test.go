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
	testNode := &Node{Commands: []Command{Command("Hello world")}}

	nodes.Put(testNode)
	got, ok := nodes.Get(testNode.Hash())

	if !ok || testNode.Commands[0] != got.Commands[0] {
		t.Errorf("Failed to retrieve node from storage.")
	}
}

// Checks that the hash function for nodes is deterministic
func TestFuzzNodeHash(t *testing.T) {
	f := fuzz.New()
	f.NilChance(0)
	for i := 0; i < 10000; i++ {
		var testNode Node
		f.Fuzz(&testNode)
		testNode.Justify = CreateQuorumCert(&testNode)
		numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
		for j := int64(0); j < numSigs.Int64(); j++ {
			var sig PartialSig
			f.Fuzz(&sig)
			id, _ := rand.Int(rand.Reader, big.NewInt(1000))
			rID := ReplicaID(id.Int64())
			sig.ID = rID
			sig.R, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			sig.S, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			testNode.Justify.Sigs[rID] = sig
		}
		hash1 := testNode.Hash()
		hash2 := testNode.Hash()
		if !bytes.Equal(hash1[:], hash2[:]) {
			t.Fatalf("Non-determinism in hash function detected:\nNode: %s\nHash1: %s\nHash2: %s", testNode, hash1, hash2)
		}
	}
}

func BenchmarkNodeHash(b *testing.B) {
	pk1, _ := GeneratePrivateKey()
	pk2, _ := GeneratePrivateKey()
	pk3, _ := GeneratePrivateKey()

	parent := &Node{Commands: []Command{"Test"}}

	node := &Node{Commands: []Command{"Hello world"}, ParentHash: parent.Hash()}

	node.Justify = CreateQuorumCert(parent)
	pc1, _ := CreatePartialCert(1, pk1, parent)
	node.Justify.AddPartial(pc1)
	pc2, _ := CreatePartialCert(2, pk2, parent)
	node.Justify.AddPartial(pc2)
	pc3, _ := CreatePartialCert(3, pk3, parent)
	node.Justify.AddPartial(pc3)

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		node.hash = nil
		b.StartTimer()
		node.Hash()
	}
}
