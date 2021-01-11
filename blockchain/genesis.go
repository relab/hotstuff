package blockchain

import "github.com/relab/hotstuff"

var genesisBlock = block{
	cert:     &genesisQC{},
	view:     0,
	proposer: 0,
}

type genesisQC struct{}

func (qc *genesisQC) ToBytes() []byte {
	return []byte{}
}

// BlockHash returns the hash of the block for which the certificate was created
func (qc *genesisQC) BlockHash() hotstuff.Hash {
	return genesisBlock.Hash()
}

// Genesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain
func GetGenesis() hotstuff.Block {
	return &genesisBlock
}
