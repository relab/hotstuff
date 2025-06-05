package modules

import (
	"context"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// HotstuffRuleset is the minimum interface that a Hotstuff variant implements.
// It must implement the vote and commit rules, but the propose rule is optional.
type HotstuffRuleset interface {
	VoteRuler
	CommitRuler
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

type VoteRuler interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(view hotstuff.View, proposal hotstuff.ProposeMsg) bool
}

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
	Verify(signature hotstuff.QuorumSignature, message []byte) error
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error
}

// Sender handles the network layer of the consensus protocol by methods for sending specific messages.
type Sender interface {
	NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error
	Vote(id hotstuff.ID, cert hotstuff.PartialCert) error
	Timeout(msg hotstuff.TimeoutMsg)
	Propose(proposal *hotstuff.ProposeMsg)
	RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool)
	Sub(ids []hotstuff.ID) (Sender, error)
}

// KauriSender is an extension of Sender allowing to send contribution messages to parent nodes.
type KauriSender interface {
	Sender
	SendContributionToParent(view hotstuff.View, qc hotstuff.QuorumSignature)
}

// ConsensusProtocol is an interface sending proposals and votes.
type ConsensusProtocol interface {
	// SendVote handles incoming proposals and replies with a vote.
	SendVote(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert)
	// SendPropose disseminates the proposal from the proposer.
	SendPropose(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert)
}

type Executor interface {
	Exec(*clientpb.Batch)
	Abort(*clientpb.Batch)
}
