package hotstuff

var genesisBlock = Block{
	cert:     nil,
	view:     0,
	proposer: 0,
}

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return &genesisBlock
}
