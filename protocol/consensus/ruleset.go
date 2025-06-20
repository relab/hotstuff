package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// VoteRuler is an interface that specifies when to vote for a block for a given view.
type VoteRuler interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(hotstuff.View, hotstuff.ProposeMsg) bool
}

// CommitRuler is an interface that specifies when to commit a block and its ancestors
// on the local chain.
type CommitRuler interface {
	// CommitRule decides whether any ancestor of the block can be committed.
	// Returns the youngest ancestor of the block that can be committed.
	CommitRule(*hotstuff.Block) *hotstuff.Block
}

// ProposeRuler is an interface that adds a ProposeRule method.
// This allows implementors to specify how new blocks are created.
type ProposeRuler interface {
	// ProposeRule creates a new proposal.
	ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (hotstuff.ProposeMsg, bool)
}

// Ruleset is the minimum interface that a Hotstuff variant implements.
// It combines VoteRuler and CommitRuler since they are mandatory. Some variants
// may implement ProposeRuler.
type Ruleset interface {
	VoteRuler
	CommitRuler
	ProposeRuler
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}
