package crypto

import "github.com/relab/hotstuff"

// Base provides the basic cryptographic methods needed to create, verify, and combine signatures.
type Base interface {
	// Sign creates a cryptographic signature of the given message.
	Sign(message []byte) (signature hotstuff.QuorumSignature, err error)
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...hotstuff.QuorumSignature) (signature hotstuff.QuorumSignature, err error)
	// Verify verifies the given quorum signature against the message.
	Verify(signature hotstuff.QuorumSignature, message []byte) error
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error
}
