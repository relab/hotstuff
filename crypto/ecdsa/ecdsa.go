// Package ecdsa provides a crypto implementation for HotStuff using Go's 'crypto/ecdsa' package.
package ecdsa

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"golang.org/x/exp/slices"
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
		order = append(order, signature.signer)
	}
	slices.Sort(order)
	for _, id := range order {
		b = append(b, sig[id].ToBytes()...)
	}
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (sig MultiSignature) Participants() hotstuff.IDSet {
	return sig
}

// Add adds an ID to the set.
func (sig MultiSignature) Add(_ hotstuff.ID) {
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

func (sig MultiSignature) String() string {
	return hotstuff.IDSetToString(sig)
}

var (
	_ hotstuff.QuorumSignature = (*MultiSignature)(nil)
	_ hotstuff.IDSet           = (*MultiSignature)(nil)
)

type ecdsaBase struct {
	configuration modules.Configuration
	logger        logging.Logger
	opts          *modules.Options
}

// New returns a new instance of the ECDSA CryptoBase implementation.
func New() modules.CryptoBase {
	return &ecdsaBase{}
}

func (ec *ecdsaBase) getPrivateKey() *ecdsa.PrivateKey {
	return ec.opts.PrivateKey().(*ecdsa.PrivateKey)
}

// InitModule gives the module a reference to the Core object.
// It also allows the module to set module options using the OptionsBuilder.
func (ec *ecdsaBase) InitModule(mods *modules.Core) {
	mods.Get(
		&ec.configuration,
		&ec.logger,
		&ec.opts,
	)
}

// Sign creates a cryptographic signature of the given message.
func (ec *ecdsaBase) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign failed: %w", err)
	}
	return MultiSignature{ec.opts.ID(): &Signature{
		r:      r,
		s:      s,
		signer: ec.opts.ID(),
	}}, nil
}

// Combine combines multiple signatures into a single signature.
func (ec *ecdsaBase) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	if len(signatures) < 2 {
		return nil, crypto.ErrCombineMultiple
	}

	ts := make(MultiSignature)

	for _, sig1 := range signatures {
		if sig2, ok := sig1.(MultiSignature); ok {
			for id, s := range sig2 {
				if _, ok := ts[id]; ok {
					return nil, crypto.ErrCombineOverlap
				}
				ts[id] = s
			}
		} else {
			ec.logger.Panicf("cannot combine signature of incompatible type %T (expected %T)", sig1, sig2)
		}
	}

	return ts, nil
}

// Verify verifies the given quorum signature against the message.
func (ec *ecdsaBase) Verify(signature hotstuff.QuorumSignature, message []byte) bool {
	s, ok := signature.(MultiSignature)
	if !ok {
		ec.logger.Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}

	n := signature.Participants().Len()
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	hash := sha256.Sum256(message)

	for _, sig := range s {
		go func(sig *Signature, hash hotstuff.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
	}

	valid := true
	for range s {
		if !<-results {
			valid = false
		}
	}

	return valid
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (ec *ecdsaBase) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool {
	s, ok := signature.(MultiSignature)
	if !ok {
		ec.logger.Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}

	n := signature.Participants().Len()
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	set := make(map[hotstuff.Hash]struct{})
	for id, sig := range s {
		message, ok := batch[id]
		if !ok {
			return false
		}
		hash := sha256.Sum256(message)
		set[hash] = struct{}{}
		go func(sig *Signature, hash hotstuff.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
	}

	valid := true
	for range s {
		if !<-results {
			valid = false
		}
	}

	// valid if all partial signatures are valid and there are no duplicate messages
	return valid && len(set) == len(batch)
}

func (ec *ecdsaBase) verifySingle(sig *Signature, hash hotstuff.Hash) bool {
	replica, ok := ec.configuration.Replica(sig.Signer())
	if !ok {
		ec.logger.Warnf("ecdsaBase: got signature from replica whose ID (%d) was not in the config.", sig.Signer())
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	return ecdsa.Verify(pk, hash[:], sig.R(), sig.S())
}
