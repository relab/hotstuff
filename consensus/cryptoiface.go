package consensus

import (
	"github.com/relab/hotstuff"
)

// CryptoBase provides the basic cryptographic methods needed to create, verify, and combine signatures.
type CryptoBase interface {
	// Sign creates a cryptographic signature of the given message.
	Sign(message []byte) (signature QuorumSignature, err error)
	// Verify verifies the given cryptographic signature according to the specified options.
	// NOTE: One of either VerifySingle or VerifyMulti options MUST be specified,
	// otherwise this function will have nothing to verify the signature against.
	Verify(signature QuorumSignature, options ...VerifyOption) bool
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...QuorumSignature) (signature QuorumSignature, err error)
}

// VerifyOption sets options for the Verify function in the CryptoBase interface.
type VerifyOption func(o *VerifyOptions)

// VerifyThreshold specifies the number of signers that must have participated
// in the signing of the signature in order for Verify to return true.
func VerifyThreshold(threshold int) VerifyOption {
	return func(o *VerifyOptions) {
		o.Threshold = threshold
	}
}

// VerifySingle verifies the signature against the given message.
//
// NOTE: VerifySingle and VerifyMulti cannot be used together,
// and whichever option is specified last will be the effective one.
func VerifySingle(message []byte) VerifyOption {
	return func(o *VerifyOptions) {
		o.Messages = map[hotstuff.ID][]byte{0: message}
	}
}

// VerifyMulti verifies the signature against the multiple messages present in the given map.
//
// NOTE: VerifySingle and VerifyMulti cannot be used together,
// and whichever option is specified last will be the effective one.
func VerifyMulti(messages map[hotstuff.ID][]byte) VerifyOption {
	return func(o *VerifyOptions) {
		o.Messages = messages
	}
}

// VerifyOptions specify options for the Verify function in the CryptoBase interface.
type VerifyOptions struct {
	Messages  map[hotstuff.ID][]byte
	Threshold int
}

// Crypto implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type Crypto interface {
	CryptoBase
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *Block) (cert PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *Block, signatures []PartialCert) (cert QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view View, timeouts []TimeoutMsg) (cert TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view View, timeouts []TimeoutMsg) (aggQC AggregateQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC AggregateQC) (ok bool, highQC QuorumCert)
}
