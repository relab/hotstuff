package modules

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/msg"
)

// CryptoBase provides the basic cryptographic methods needed to create, verify, and combine signatures.
type CryptoBase interface {
	// Sign creates a cryptographic signature of the given message.
	Sign(message []byte) (signature *hotstuffpb.QuorumSignature, err error)
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...*hotstuffpb.QuorumSignature) (signature *hotstuffpb.QuorumSignature, err error)
	// Verify verifies the given quorum signature against the message.
	Verify(signature *hotstuffpb.QuorumSignature, message []byte) bool
	// BatchVerify verifies the given quorum signature against the batch of messages.
	BatchVerify(signature *hotstuffpb.QuorumSignature, batch map[hotstuff.ID][]byte) bool
}

// Crypto implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type Crypto interface {
	CryptoBase
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *msg.Block) (cert hotstuffpb.PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *msg.Block, signatures []hotstuffpb.PartialCert) (cert msg.QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view msg.View, timeouts []msg.TimeoutMsg) (cert msg.TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view msg.View, timeouts []msg.TimeoutMsg) (aggQC msg.AggregateQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert hotstuffpb.PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc msg.QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc msg.TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC msg.AggregateQC) (highQC msg.QuorumCert, ok bool)
}
