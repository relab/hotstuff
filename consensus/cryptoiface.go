package consensus

import (
	"github.com/relab/hotstuff"
)

// CryptoBase provides the basic cryptographic methods needed to create, verify, and combine signatures.
type CryptoBase interface {
	// Sign creates a cryptographic signature of the given hash.
	Sign(hash Hash) (signature QuorumSignature, err error)
	// Verify verifies the given cryptographic signature according to the specified options.
	// NOTE: One of either VerifyHash or VerifyHashes MUST be specified,
	// otherwise this function will have nothing to verify the signature against.
	Verify(signature QuorumSignature, options ...VerifyOption) bool
	// Combine combines multiple signatures into a single signature.
	Combine(signatures ...QuorumSignature) QuorumSignature
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

// VerifyHash verifies the signature against the given hash.
//
// NOTE: VerifyHash and VerifyHashes cannot be used together,
// and whichever option is specified last will be the effective one.
func VerifyHash(hash Hash) VerifyOption {
	return func(o *VerifyOptions) {
		o.UseHashMap = false
		o.Hash = &hash
	}
}

// VerifyHashes verifies the signature against the hashes present in the given map.
//
// NOTE: VerifyHash and VerifyHashes cannot be used together,
// and whichever option is specified last will be the effective one.
func VerifyHashes(hashMap map[hotstuff.ID]Hash) VerifyOption {
	return func(o *VerifyOptions) {
		o.UseHashMap = true
		o.HashMap = hashMap
	}
}

// VerifyOptions specify options for the Verify function in the CryptoBase interface.
type VerifyOptions struct {
	// using a pointer for the Hash such that we can tell if it is unspecified

	Hash       *Hash
	HashMap    map[hotstuff.ID]Hash
	UseHashMap bool // true if `HashMap` should be used instead of `Hash`
	Threshold  int
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

// CryptoImpl implements only the cryptographic primitives that are needed for HotStuff.
// This interface is implemented by the ecdsa and bls12 packages.
//
// Deprecated: Use CryptoBase instead.
type CryptoImpl interface {
	// Sign signs a hash.
	Sign(hash Hash) (sig Signature, err error)
	// Verify verifies a signature given a hash.
	Verify(sig Signature, hash Hash) bool
	// VerifyAggregateSignature verifies an aggregated signature.
	// It does not check whether the aggregated signature contains a quorum of signatures.
	VerifyAggregateSignature(agg QuorumSignature, hash Hash) bool
	// CreateThresholdSignature creates a threshold signature from the given partial signatures.
	CreateThresholdSignature(partialSignatures []Signature, hash Hash) (QuorumSignature, error)
	// CreateThresholdSignatureForMessageSet creates a threshold signature where each partial signature has signed a
	// different message hash.
	CreateThresholdSignatureForMessageSet(partialSignatures []Signature, hashes map[hotstuff.ID]Hash) (QuorumSignature, error)
	// VerifyThresholdSignature verifies a threshold signature.
	VerifyThresholdSignature(signature QuorumSignature, hash Hash) bool
	// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
	VerifyThresholdSignatureForMessageSet(signature QuorumSignature, hashes map[hotstuff.ID]Hash) bool
	// Combine combines multiple signatures into a single threshold signature.
	// Arguments can be singular signatures or threshold signatures.
	//
	// As opposed to the CreateThresholdSignature methods,
	// this method does not check whether the resulting
	// signature meets the quorum size.
	Combine(signatures ...interface{}) QuorumSignature
}
