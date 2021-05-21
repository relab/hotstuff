package consensus

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
