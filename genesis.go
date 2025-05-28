package hotstuff

var genesisBlock = GenesisBlock()

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
