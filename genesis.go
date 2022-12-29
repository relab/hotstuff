package hotstuff

import "time"

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0, time.Now())

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
