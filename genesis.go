package hotstuff

//var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0, 1)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis(chainNumber ChainNumber) *Block {

	return NewBlock(Hash{}, QuorumCert{}, "", 0, 0, chainNumber)
}
