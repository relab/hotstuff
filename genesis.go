package hotstuff

import "time"

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	ts := time.Unix(0, 0).UTC() // should we use a static string for parsing?
	genesisBlock.SetTimestamp(ts)
	return genesisBlock
}
