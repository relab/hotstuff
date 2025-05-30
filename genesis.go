package hotstuff

import "github.com/relab/hotstuff/internal/proto/clientpb"

var genesisBlock = NewBlock(Hash{}, QuorumCert{}, &clientpb.Batch{}, 0, 0)

// GetGenesis returns a pointer to the genesis block, the starting point for the hotstuff blockchain.
func GetGenesis() *Block {
	return genesisBlock
}
