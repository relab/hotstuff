// Package ecdsa provides a crypto implementation for HotStuff using Go's 'crypto/ecdsa' package.
package ecdsa

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/modules"
	"go.uber.org/multierr"
)

func init() {
	modules.RegisterModule("ecdsa", New)
}

const (
	// PrivateKeyFileType is the PEM type for a private key.
	PrivateKeyFileType = "ECDSA PRIVATE KEY"

	// PublicKeyFileType is the PEM type for a public key.
	PublicKeyFileType = "ECDSA PUBLIC KEY"
)

// Signature is an ECDSA signature
type Signature struct {
	r, s   *big.Int
	signer hotstuff.ID
}

// RestoreSignature restores an existing signature. It should not be used to create new signatures, use Sign instead.
func RestoreSignature(r, s *big.Int, signer hotstuff.ID) *Signature {
	return &Signature{r, s, signer}
}

// Signer returns the ID of the replica that generated the signature.
func (sig Signature) Signer() hotstuff.ID {
	return sig.signer
}

// R returns the r value of the signature
func (sig Signature) R() *big.Int {
	return sig.r
}

// S returns the s value of the signature
func (sig Signature) S() *big.Int {
	return sig.s
}

// ToBytes returns a raw byte string representation of the signature
func (sig Signature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.r.Bytes()...)
	b = append(b, sig.s.Bytes()...)
	return b
}

var _ consensus.Signature = (*Signature)(nil)

// ThresholdSignature is a set of (partial) signatures that form a valid threshold signature when there are a quorum
// of valid (partial) signatures.
type ThresholdSignature map[hotstuff.ID]*Signature

// RestoreThresholdSignature should only be used to restore an existing threshold signature from a set of signatures.
// To create a new verifiable threshold signature, use CreateThresholdSignature instead.
func RestoreThresholdSignature(signatures []*Signature) ThresholdSignature {
	sig := make(ThresholdSignature, len(signatures))
	for _, s := range signatures {
		sig[s.signer] = s
	}
	return sig
}

// ToBytes returns the object as bytes.
func (sig ThresholdSignature) ToBytes() []byte {
	var b []byte
	// sort by ID to make it deterministic
	order := make([]hotstuff.ID, 0, len(sig))
	for _, signature := range sig {
		i := sort.Search(len(order), func(i int) bool { return signature.signer < order[i] })
		order = append(order, 0)
		copy(order[i+1:], order[i:])
		order[i] = signature.signer
	}
	for _, id := range order {
		b = append(b, sig[id].ToBytes()...)
	}
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (sig ThresholdSignature) Participants() consensus.IDSet {
	return sig
}

// Add adds an ID to the set.
func (sig ThresholdSignature) Add(id hotstuff.ID) {
	panic("not implemented")
}

// Contains returns true if the set contains the ID.
func (sig ThresholdSignature) Contains(id hotstuff.ID) bool {
	_, ok := sig[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (sig ThresholdSignature) ForEach(f func(hotstuff.ID)) {
	for id := range sig {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (sig ThresholdSignature) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range sig {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (sig ThresholdSignature) Len() int {
	return len(sig)
}

var _ consensus.ThresholdSignature = (*ThresholdSignature)(nil)
var _ consensus.IDSet = (*ThresholdSignature)(nil)

type ecdsaCrypto struct {
	mods *consensus.Modules
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (ec *ecdsaCrypto) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	ec.mods = mods
}

// New returns a new signer and a new verifier.
func New() consensus.CryptoImpl {
	ec := &ecdsaCrypto{}
	return ec
}

func (ec *ecdsaCrypto) getPrivateKey() *ecdsa.PrivateKey {
	pk := ec.mods.PrivateKey()
	return pk.(*ecdsa.PrivateKey)
}

// Sign signs a hash.
func (ec *ecdsaCrypto) Sign(hash consensus.Hash) (sig consensus.Signature, err error) {
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign failed: %w", err)
	}
	return &Signature{
		r:      r,
		s:      s,
		signer: ec.mods.ID(),
	}, nil
}

// Verify verifies a signature given a hash.
func (ec *ecdsaCrypto) Verify(sig consensus.Signature, hash consensus.Hash) bool {
	_sig, ok := sig.(*Signature)
	if !ok {
		return false
	}
	replica, ok := ec.mods.Configuration().Replica(sig.Signer())
	if !ok {
		ec.mods.Logger().Infof("ecdsaCrypto: got signature from replica whose ID (%d) was not in the config.", sig.Signer())
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	return ecdsa.Verify(pk, hash[:], _sig.R(), _sig.S())
}

// VerifyAggregateSignature verifies an aggregated signature.
// It does not check whether the aggregated signature contains a quorum of signatures.
func (ec *ecdsaCrypto) VerifyAggregateSignature(agg consensus.ThresholdSignature, hash consensus.Hash) bool {
	sig, ok := agg.(ThresholdSignature)
	if !ok {
		return false
	}
	results := make(chan bool)
	for _, pSig := range sig {
		go func(sig *Signature) {
			results <- ec.mods.Crypto().Verify(sig, hash)
		}(pSig)
	}
	valid := true
	for range sig {
		if <-results {
			valid = false
		}
	}
	return valid
}

// CreateThresholdSignature creates a threshold signature from the given partial signatures.
func (ec *ecdsaCrypto) CreateThresholdSignature(partialSignatures []consensus.Signature, hash consensus.Hash) (_ consensus.ThresholdSignature, err error) {
	thrSig := make(ThresholdSignature)
	for _, s := range partialSignatures {
		if thrSig.Participants().Contains(s.Signer()) {
			err = multierr.Append(err, crypto.ErrPartialDuplicate)
			continue
		}

		sig, ok := s.(*Signature)
		if !ok {
			err = multierr.Append(err, fmt.Errorf("%w: %T", crypto.ErrWrongType, s))
			continue
		}

		// use the registered verifier instead of ourself to verify.
		// this makes it possible for the signatureCache to work.
		if ec.mods.Crypto().Verify(s, hash) {
			thrSig[sig.signer] = sig
		}
	}

	if len(thrSig) >= ec.mods.Configuration().QuorumSize() {
		return thrSig, nil
	}

	return nil, multierr.Combine(crypto.ErrNotAQuorum, err)
}

// CreateThresholdSignatureForMessageSet creates a ThresholdSignature of partial signatures where each partialSignature
// has signed a different message hash.
func (ec *ecdsaCrypto) CreateThresholdSignatureForMessageSet(partialSignatures []consensus.Signature, hashes map[hotstuff.ID]consensus.Hash) (_ consensus.ThresholdSignature, err error) {
	ec.mods.Logger().Debug(hashes)
	thrSig := make(ThresholdSignature)
	for _, s := range partialSignatures {
		if thrSig.Participants().Contains(s.Signer()) {
			err = multierr.Append(err, crypto.ErrPartialDuplicate)
			continue
		}

		hash, ok := hashes[s.Signer()]
		if !ok {
			continue
		}

		sig, ok := s.(*Signature)
		if !ok {
			err = multierr.Append(err, fmt.Errorf("%w: %T", crypto.ErrWrongType, s))
			continue
		}

		// use the registered verifier instead of ourself to verify.
		// this makes it possible for the signatureCache to work.
		if ec.mods.Crypto().Verify(s, hash) {
			thrSig[sig.signer] = sig
		}
	}

	if len(thrSig) >= ec.mods.Configuration().QuorumSize() {
		return thrSig, nil
	}

	return nil, multierr.Combine(crypto.ErrNotAQuorum, err)
}

// VerifyThresholdSignature verifies a threshold signature.
func (ec *ecdsaCrypto) VerifyThresholdSignature(signature consensus.ThresholdSignature, hash consensus.Hash) bool {
	sig, ok := signature.(ThresholdSignature)
	if !ok {
		return false
	}
	if len(sig) < ec.mods.Configuration().QuorumSize() {
		return false
	}
	results := make(chan bool)
	for _, pSig := range sig {
		go func(sig *Signature) {
			results <- ec.mods.Crypto().Verify(sig, hash)
		}(pSig)
	}
	numVerified := 0
	for range sig {
		if <-results {
			numVerified++
		}
	}
	return numVerified >= ec.mods.Configuration().QuorumSize()
}

// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
func (ec *ecdsaCrypto) VerifyThresholdSignatureForMessageSet(signature consensus.ThresholdSignature, hashes map[hotstuff.ID]consensus.Hash) bool {
	ec.mods.Logger().Debug(hashes)
	sig, ok := signature.(ThresholdSignature)
	if !ok {
		return false
	}
	hashSet := make(map[consensus.Hash]struct{})
	results := make(chan bool)
	for id, hash := range hashes {
		if _, ok := hashSet[hash]; ok {
			return false
		}
		hashSet[hash] = struct{}{}
		s, ok := sig[id]
		if !ok {
			return false
		}
		go func(sig *Signature, hash consensus.Hash) {
			results <- ec.mods.Crypto().Verify(sig, hash)
		}(s, hash)
	}
	numVerified := 0
	for range sig {
		if <-results {
			numVerified++
		}
	}
	return numVerified >= ec.mods.Configuration().QuorumSize()
}

// Combine combines multiple signatures into a single threshold signature.
// Arguments can be singular signatures or threshold signatures.
//
// As opposed to the CreateThresholdSignature methods,
// this method does not check whether the resulting
// signature meets the quorum size.
func (ec *ecdsaCrypto) Combine(signatures ...interface{}) consensus.ThresholdSignature {
	ts := make(ThresholdSignature)

	for _, sig := range signatures {
		switch sig := sig.(type) {
		case *Signature:
			ts[sig.signer] = sig
		case ThresholdSignature:
			for _, s := range sig {
				ts[s.signer] = s
			}
		}
	}

	return ts
}

var _ consensus.CryptoImpl = (*ecdsaCrypto)(nil)
