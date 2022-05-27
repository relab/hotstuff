package modules

import "github.com/relab/hotstuff"

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

// Crypto implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type Crypto interface {
	CryptoBase
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (aggQC hotstuff.AggregateQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert hotstuff.PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc hotstuff.QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc hotstuff.TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC hotstuff.AggregateQC) (highQC hotstuff.QuorumCert, ok bool)
}
