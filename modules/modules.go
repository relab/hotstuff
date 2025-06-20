package modules

import (
	"context"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// VoteRuler is an interface that specifies when to vote for a block for a given view.
type VoteRuler interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(view hotstuff.View, proposal hotstuff.ProposeMsg) bool
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
	ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool)
}

// HotstuffRuleset is the minimum interface that a Hotstuff variant implements.
// It combines VoteRuler and CommitRuler since they are mandatory. Some variants
// may implement ProposeRuler.
type HotstuffRuleset interface {
	VoteRuler
	CommitRuler
	ProposeRuler
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(hotstuff.View) hotstuff.ID
}

// Disseminator is an interface for disseminating the proposal from the proposer.
type Disseminator interface {
	// Disseminate disseminates the proposal from the proposer.
	Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error
}

// Aggregator is an interface for handling incoming proposals and replying with a vote.
type Aggregator interface {
	// Aggregate handles incoming proposals and replies with a vote.
	Aggregate(lastVote hotstuff.View, proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error
}

// DisseminatorAggregator is an interface that combines Disseminator and Aggregator for convenience.
type DisseminatorAggregator interface {
	Disseminator
	Aggregator
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
	Verify(signature hotstuff.QuorumSignature, message []byte) error
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error
}

// Sender handles the network layer of the consensus protocol by methods for sending specific messages.
type Sender interface {
	// NewView sends a new view message to a replica. Returns an error if the replica was not found.
	NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error
	// Vote sends a vote message to a replica. Returns an error if the replica was not found.
	Vote(id hotstuff.ID, cert hotstuff.PartialCert) error
	// Timeout broadcasts a timeout message to the replicas.
	Timeout(msg hotstuff.TimeoutMsg)
	// Propose broadcasts a propose message to the replicas.
	Propose(proposal *hotstuff.ProposeMsg)
	// RequestBlock sends a request to the replicas to send back a locally missing block.
	RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool)
	// Sub returns a new sender copy that is only allowed to send to the provided ids.
	// Returns an error if the ids are not a subset of the parent's ids.
	Sub(ids []hotstuff.ID) (Sender, error)
}

// KauriSender is an extension of Sender allowing to send contribution messages to parent nodes.
type KauriSender interface {
	Sender
	// SendContributionToParent aggregates the contribution to the parent.
	SendContributionToParent(view hotstuff.View, qc hotstuff.QuorumSignature)
}
