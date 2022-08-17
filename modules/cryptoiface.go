package modules

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/msg"
)

// CryptoBase provides the basic cryptographic methods needed to create, verify, and combine signatures.
type CryptoBase interface {
	// Sign creates a cryptographic signature of the given message.
	Sign(message []byte) (signature *msg.Signature, err error)
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...*msg.ThresholdSignature) (signature *msg.ThresholdSignature, err error)
	// Verify verifies the given quorum signature against the message.
	Verify(signature *msg.ThresholdSignature, message []byte) bool
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature *msg.ThresholdSignature, batch map[hotstuff.ID][]byte) bool
}

// Crypto implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type Crypto interface {
	CryptoBase
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *msg.Block) (cert *msg.PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *msg.Block, signatures []*msg.PartialCert) (cert *msg.QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view msg.View, timeouts []*msg.TimeoutMsg) (cert *msg.TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view msg.View, timeouts []*msg.TimeoutMsg) (aggQC *msg.AggQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert *msg.PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc *msg.QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc *msg.TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC *msg.AggQC) (highQC *msg.QuorumCert, ok bool)
}
