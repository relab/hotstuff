package data

import (
	"bytes"
	"crypto/rand"
	"math"
	"math/big"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/relab/hotstuff/config"
)

func TestCanStoreAndRetrieveBlock(t *testing.T) {
	blocks := NewMapStorage()
	testBlock := &Block{Commands: []Command{Command("Hello world")}}

	blocks.Put(testBlock)
	got, ok := blocks.Get(testBlock.Hash())

	if !ok || testBlock.Commands[0] != got.Commands[0] {
		t.Errorf("Failed to retrieve block from storage.")
	}
}

// Checks that the hash function for blocks is deterministic
func TestFuzzBlockHash(t *testing.T) {
	f := fuzz.New()
	f.NilChance(0)
	for i := 0; i < 10000; i++ {
		var testBlock Block
		f.Fuzz(&testBlock)
		testBlock.Justify = CreateQuorumCert(&testBlock)
		numSigs, _ := rand.Int(rand.Reader, big.NewInt(10))
		for j := int64(0); j < numSigs.Int64(); j++ {
			var sig PartialSig
			f.Fuzz(&sig)
			id, _ := rand.Int(rand.Reader, big.NewInt(1000))
			rID := config.ReplicaID(id.Int64())
			sig.ID = rID
			sig.R, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			sig.S, _ = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			testBlock.Justify.Sigs[rID] = sig
		}
		hash1 := testBlock.Hash()
		hash2 := testBlock.Hash()
		if !bytes.Equal(hash1[:], hash2[:]) {
			t.Fatalf("Non-determinism in hash function detected:\nBlock: %s\nHash1: %s\nHash2: %s", testBlock, hash1, hash2)
		}
	}
}

func BenchmarkBlockHash(b *testing.B) {
	pk1, _ := GeneratePrivateKey()
	pk2, _ := GeneratePrivateKey()
	pk3, _ := GeneratePrivateKey()

	parent := &Block{Commands: []Command{"Test"}}

	block := &Block{Commands: []Command{"Hello world"}, ParentHash: parent.Hash()}

	block.Justify = CreateQuorumCert(parent)
	pc1, _ := CreatePartialCert(1, pk1, parent)
	block.Justify.AddPartial(pc1)
	pc2, _ := CreatePartialCert(2, pk2, parent)
	block.Justify.AddPartial(pc2)
	pc3, _ := CreatePartialCert(3, pk3, parent)
	block.Justify.AddPartial(pc3)

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		block.hash = nil
		b.StartTimer()
		block.Hash()
	}
}
