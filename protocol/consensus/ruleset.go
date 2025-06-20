package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// VoteRuler is the interface that wraps the VoteRule method.
//
// The VoteRule method decides whether or not to vote for a block in a given view.
type VoteRuler interface {
	// VoteRule returns true if the proposal's block should be voted for in the given view.
	VoteRule(hotstuff.View, hotstuff.ProposeMsg) bool
}

// CommitRuler is the interface that wraps the CommitRule method.
//
// The CommitRule method is used to determine the youngest ancestor of a given block
// that can be committed on the local chain.
type CommitRuler interface {
	// CommitRule returns the youngest ancestor of the given block that can be committed.
	CommitRule(*hotstuff.Block) *hotstuff.Block
}

// ProposeRuler is the interface that wraps the ProposeRule method.
//
// This allows implementors to specify how new blocks are created.
type ProposeRuler interface {
	// ProposeRule creates a new proposal.
	ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (hotstuff.ProposeMsg, bool)
}

// Ruleset is the interface that groups the VoteRule, CommitRule, ProposeRule, and ChainLength methods.
//
// Ruleset is the minimum interface that a consensus protocol must implement and defines the
// rules for voting, committing, proposing, and the chain length required for committing blocks.
type Ruleset interface {
	VoteRuler
	CommitRuler
	ProposeRuler
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}
