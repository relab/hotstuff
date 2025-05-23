package modules

import (
	"context"
	"time"

	"github.com/relab/hotstuff"
)

// ConsensusRules is the minimum interface that a consensus implementations must implement.
// Implementations of this interface can be wrapped in the ConsensusBase struct.
// Together, these provide an implementation of the main Consensus interface.
// Implementors do not need to verify certificates or interact with other modules,
// as this is handled by the ConsensusBase struct.
type ConsensusRules interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(view hotstuff.View, proposal hotstuff.ProposeMsg) bool
	// CommitRule decides whether any ancestor of the block can be committed.
	// Returns the youngest ancestor of the block that can be committed.
	CommitRule(*hotstuff.Block) *hotstuff.Block
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

// ProposeRuler is an optional interface that adds a ProposeRule method.
// This allows implementors to specify how new blocks are created.
type ProposeRuler interface {
	// ProposeRule creates a new proposal.
	ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool)
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(hotstuff.View) hotstuff.ID
	// ViewDuration returns an object that determines the duration of a view.
	ViewDuration() ViewDuration
}

// ViewDuration determines the duration of a view.
// The view synchronizer uses this interface to set its timeouts.
type ViewDuration interface {
	// Duration returns the duration that the next view should last.
	Duration() time.Duration
	// ViewStarted is called by the synchronizer when starting a new view.
	ViewStarted()
	// ViewSucceeded is called by the synchronizer when a view ended successfully.
	ViewSucceeded()
	// ViewTimeout is called by the synchronizer when a view timed out.
	ViewTimeout()
}

// CryptoBase provides the basic cryptographic methods needed to create, verify, and combine signatures.
type CryptoBase interface {
	// Sign creates a cryptographic signature of the given message.
	Sign(message []byte) (signature hotstuff.QuorumSignature, err error)
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...hotstuff.QuorumSignature) (signature hotstuff.QuorumSignature, err error)
	// Verify verifies the given quorum signature against the message.
	Verify(signature hotstuff.QuorumSignature, message []byte) bool
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool
}

// Sender handles the network layer of the consensus protocol by methods for sending specific messages.
type Sender interface {
	NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error
	Vote(id hotstuff.ID, cert hotstuff.PartialCert) error
	Timeout(msg hotstuff.TimeoutMsg)
	Propose(proposal *hotstuff.ProposeMsg)
	RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool)
}

// ConsensusSender is an extension for handling proposals as either the voter or proposer.
type ConsensusSender interface {
	// SendVote handles incoming proposals and replies with a vote.
	SendVote(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert)
	// SendPropose disseminates the proposal from the proposer.
	SendPropose(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert)
}
