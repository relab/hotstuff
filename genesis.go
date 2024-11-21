package hotstuff

// Genesis block can be on pipe 1, even if no pipelining is enabled
const firstPipe = 1

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0, firstPipe)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
