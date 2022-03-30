// Package ecdsa provides a crypto implementation for HotStuff using Go's 'crypto/ecdsa' package.
package ecdsa

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sort"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
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

// MultiSignature is a set of (partial) signatures.
type MultiSignature map[hotstuff.ID]*Signature

// RestoreMultiSignature should only be used to restore an existing threshold signature from a set of signatures.
func RestoreMultiSignature(signatures []*Signature) MultiSignature {
	sig := make(MultiSignature, len(signatures))
	for _, s := range signatures {
		sig[s.signer] = s
	}
	return sig
}

// ToBytes returns the object as bytes.
func (sig MultiSignature) ToBytes() []byte {
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
func (sig MultiSignature) Participants() consensus.IDSet {
	return sig
}

// Add adds an ID to the set.
func (sig MultiSignature) Add(id hotstuff.ID) {
	panic("not implemented")
}

// Contains returns true if the set contains the ID.
func (sig MultiSignature) Contains(id hotstuff.ID) bool {
	_, ok := sig[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (sig MultiSignature) ForEach(f func(hotstuff.ID)) {
	for id := range sig {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (sig MultiSignature) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range sig {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (sig MultiSignature) Len() int {
	return len(sig)
}

var _ consensus.QuorumSignature = (*MultiSignature)(nil)
var _ consensus.IDSet = (*MultiSignature)(nil)

type ecdsaBase struct {
	mods *consensus.Modules
}

// New returns a new instance of the ECDSA CryptoBase implementation.
func New() consensus.CryptoBase {
	return &ecdsaBase{}
}

func (ec *ecdsaBase) getPrivateKey() *ecdsa.PrivateKey {
	pk := ec.mods.PrivateKey()
	return pk.(*ecdsa.PrivateKey)
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (ec *ecdsaBase) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	ec.mods = mods
}

// Sign creates a cryptographic signature of the given message.
func (ec *ecdsaBase) Sign(message []byte) (signature consensus.QuorumSignature, err error) {
	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign failed: %w", err)
	}
	return MultiSignature{ec.mods.ID(): &Signature{
		r:      r,
		s:      s,
		signer: ec.mods.ID(),
	}}, nil
}

// Verify verifies the given cryptographic signature according to the specified options.
// NOTE: One of either VerifySingle or VerifyMulti options MUST be specified,
// otherwise this function will have nothing to verify the signature against.
func (ec *ecdsaBase) Verify(signature consensus.QuorumSignature, options ...consensus.VerifyOption) bool {
	sig, ok := signature.(MultiSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify signature of incompatible type %T (expected %T)", signature, sig))
	}

	var opts consensus.VerifyOptions
	for _, opt := range options {
		opt(&opts)
	}

	if len(opts.Messages) == 0 {
		panic("no message(s) to verify the signature against: you must specify one of the VerifySingle or VerifyMulti options")
	}

	if signature.Participants().Len() < opts.Threshold {
		return false
	}

	results := make(chan bool)
	for _, pSig := range sig {
		var hash consensus.Hash

		if len(opts.Messages) == 1 {
			hash = sha256.Sum256(opts.Messages[0])
		} else {
			hash = sha256.Sum256(opts.Messages[pSig.signer])
		}

		go func(sig *Signature, hash consensus.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(pSig, hash)
	}

	valid := true
	for range sig {
		if !<-results {
			valid = false
		}
	}

	return valid
}

func (ec *ecdsaBase) verifySingle(sig *Signature, hash consensus.Hash) bool {
	replica, ok := ec.mods.Configuration().Replica(sig.Signer())
	if !ok {
		ec.mods.Logger().Infof("ecdsaBase: got signature from replica whose ID (%d) was not in the config.", sig.Signer())
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	return ecdsa.Verify(pk, hash[:], sig.R(), sig.S())
}

// Combine combines multiple signatures into a single signature.
func (ec *ecdsaBase) Combine(signatures ...consensus.QuorumSignature) (consensus.QuorumSignature, error) {
	ts := make(MultiSignature)

	for _, sig := range signatures {
		if sig, ok := sig.(MultiSignature); ok {
			for id, s := range sig {
				ts[id] = s
			}
		}
	}

	return ts, nil
}
