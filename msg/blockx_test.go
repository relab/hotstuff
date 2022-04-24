package msg

import (
	"testing"

	"github.com/relab/hotstuff/msg/hotstuffpb"
)

var (
	block *Block
	x     *BlockX
	y     *BlockY
	pb    *hotstuffpb.Block
	buf   []byte
)

func BenchmarkNewBlock(b *testing.B) {
	qc := NewQuorumCert(nil, 0, Hash{})
	genesisBlock := GetGenesis().Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = NewBlock(genesisBlock, qc, "command", 1, 1)
	}
}

func BenchmarkNewBlockX(b *testing.B) {
	qc := &hotstuffpb.QuorumCert{Sig: nil, View: 0, Hash: []byte("hash")}
	genesisBlock := GetGenesis().Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x = NewBlockX(genesisBlock, qc, "command", 1, 1)
	}
}

func BenchmarkNewBlockY(b *testing.B) {
	qc := &hotstuffpb.QuorumCert{Sig: nil, View: 0, Hash: []byte("hash")}
	genesisBlock := GetGenesis().Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		y = NewBlockY(genesisBlock, qc, "command", 1, 1)
	}
}

func BenchmarkNewBlockProto(b *testing.B) {
	qc := &hotstuffpb.QuorumCert{Sig: nil, View: 0, Hash: []byte("hash")}
	genesisBlock := GetGenesis().Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pb = &hotstuffpb.Block{
			Parent:   genesisBlock[:],
			QC:       qc,
			Command:  []byte("command"),
			View:     1,
			Proposer: 1,
		}
	}
}

func BenchmarkToBytes(b *testing.B) {
	qc := NewQuorumCert(nil, 0, Hash{})
	genesisBlock := GetGenesis().Hash()
	block := NewBlock(genesisBlock, qc, "command", 1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = block.ToBytes()
	}
}

func BenchmarkToBytesX(b *testing.B) {
	qc := &hotstuffpb.QuorumCert{Sig: nil, View: 0, Hash: []byte("hash")}
	genesisBlock := GetGenesis().Hash()
	block := NewBlockX(genesisBlock, qc, "command", 1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = block.ToBytesX()
	}
}

func BenchmarkToBytesY(b *testing.B) {
	qc := &hotstuffpb.QuorumCert{Sig: nil, View: 0, Hash: []byte("hash")}
	genesisBlock := GetGenesis().Hash()
	block := NewBlockY(genesisBlock, qc, "command", 1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = block.ToBytesY()
	}
}
