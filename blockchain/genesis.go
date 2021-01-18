package blockchain

import "github.com/relab/hotstuff"

var genesisBlock = block{
	cert:     nil,
	view:     0,
	proposer: 0,
}

// Genesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain
func GetGenesis() hotstuff.Block {
	return &genesisBlock
}
