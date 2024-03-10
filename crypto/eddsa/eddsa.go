// Package eddsa implements the ed25519 curve signature.
package eddsa

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("eddsa", New)
}

const (
	// PrivateKeyFileType is the PEM type for a private key.
	PrivateKeyFileType = "EDDSA PRIVATE KEY"
	// PublicKeyFileType is the PEM type for a public key.
	PublicKeyFileType = "EDDSA PUBLIC KEY"
)

var (
	_ hotstuff.QuorumSignature = (*crypto.Multi[*Signature])(nil)
	_ hotstuff.IDSet           = (*crypto.Multi[*Signature])(nil)
	_ crypto.Signature         = (*Signature)(nil)
)

// Signature is an EDDSA signature.
type Signature struct {
	signer hotstuff.ID
	sign   []byte
}

// RestoreSignature restores an existing signature.
// It should not be used to create new signatures, use Sign instead.
func RestoreSignature(sign []byte, signer hotstuff.ID) *Signature {
	return &Signature{signer, sign}
}

// Signer returns the ID of the replica that generated the signature.
func (sig Signature) Signer() hotstuff.ID {
	return sig.signer
}

// ToBytes returns a raw byte string representation of the signature.
func (sig Signature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.sign...)
	return b
}

type eddsaBase struct {
	configuration modules.Configuration
	logger        logging.Logger
	opts          *modules.Options
}

// New returns a new instance of the EDDSA CryptoBase implementation.
func New() modules.CryptoBase {
	return &eddsaBase{}
}

// InitModule gives the module a reference to the Core object.
// It also allows the module to set module options using the OptionsBuilder.
func (ed *eddsaBase) InitModule(mods *modules.Core) {
	mods.Get(
		&ed.configuration,
		&ed.logger,
		&ed.opts,
	)
}

func (ed *eddsaBase) privateKey() ed25519.PrivateKey {
	return ed.opts.PrivateKey().(ed25519.PrivateKey)
}

// Sign creates a cryptographic signature of the given message.
func (ed *eddsaBase) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	sign := ed25519.Sign(ed.privateKey(), message)
	eddsaSign := &Signature{signer: ed.opts.ID(), sign: sign}
	return crypto.Multi[*Signature]{ed.opts.ID(): eddsaSign}, nil
}

// Combine combines multiple signatures into a single signature.
func (ed *eddsaBase) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	if len(signatures) < 2 {
		return nil, crypto.ErrCombineMultiple
	}

	ts := make(crypto.Multi[*Signature])
	for _, sig1 := range signatures {
		if sig2, ok := sig1.(crypto.Multi[*Signature]); ok {
			for id, s := range sig2 {
				if _, duplicate := ts[id]; duplicate {
					return nil, crypto.ErrCombineOverlap
				}
				ts[id] = s
			}
		} else {
			ed.logger.Panicf("cannot combine signature of incompatible type %T (expected %T)", sig1, sig2)
		}
	}
	return ts, nil
}

// Verify verifies the given quorum signature against the message.
func (ed *eddsaBase) Verify(signature hotstuff.QuorumSignature, message []byte) bool {
	s, ok := signature.(crypto.Multi[*Signature])
	if !ok {
		ed.logger.Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	n := signature.Participants().Len()
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	for _, sig := range s {
		go func(sig *Signature, msg []byte) {
			results <- ed.verifySingle(sig, msg)
		}(sig, message)
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
func (ed *eddsaBase) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool {
	s, ok := signature.(crypto.Multi[*Signature])
	if !ok {
		ed.logger.Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
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
		go func(sig *Signature, msg []byte) {
			results <- ed.verifySingle(sig, msg)
		}(sig, message)
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

func (ed *eddsaBase) verifySingle(sig *Signature, message []byte) bool {
	replica, ok := ed.configuration.Replica(sig.Signer())
	if !ok {
		ed.logger.Warnf("eddsaBase: got signature from replica whose ID (%d) was not in the config.", sig.Signer())
		return false
	}
	pk := replica.PublicKey().(ed25519.PublicKey)
	return ed25519.Verify(pk, message, sig.sign)
}
