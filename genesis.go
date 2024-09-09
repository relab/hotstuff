package hotstuff

// TODO: Verify if genesis block needs to be on pipe 1
const firstPipe = 1

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, "", 0, 0, firstPipe)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
