package hotstuffpb

import "github.com/relab/hotstuff"

// ProposerID returns the block proposer's ID.
func (x *Proposal) ProposerID() hotstuff.ID {
	if x != nil {
		return hotstuff.ID(x.GetBlock().GetProposer())
	}
	return 0
}

// ProposerID returns the ID of the replica who proposed the block.
func (b *Block) ProposerID() hotstuff.ID {
	return hotstuff.ID(b.GetProposer())
}
