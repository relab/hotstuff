package core

import (
	"context"

	"github.com/relab/hotstuff"
)

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
