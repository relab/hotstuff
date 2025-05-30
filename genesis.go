package hotstuff

import "time"

// genesisBlock is initialized at package initialization time.
var genesisBlock = func() *Block {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	b := NewBlock(Hash{}, QuorumCert{}, "", 0, 0)
	b.SetTimestamp(ts)
	return b
}()

// GetGenesis returns the genesis block.
func GetGenesis() *Block {
	return genesisBlock
}
