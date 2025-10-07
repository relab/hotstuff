// Package crypto implements several signature schemes.
// It provides a common interface for creating, verifying, and combining signatures.
// The supported schemes are:
// - EDDSA (Ed25519)
// - ECDSA (secp256k1)
// - BLS12 (BLS signatures)
package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const NameEDDSA = "eddsa"

const (
	// PrivateKeyFileType is the PEM type for a private key.
	EDDSAPrivateKeyFileType = "EDDSA PRIVATE KEY"
	// PublicKeyFileType is the PEM type for a public key.
	EDDSAPublicKeyFileType = "EDDSA PUBLIC KEY"
)

// EDDSASignature is an EDDSA signature.
type EDDSASignature struct {
	signer hotstuff.ID
	sign   []byte
}

// RestoreEDDSASignature restores an existing signature.
// It should not be used to create new signatures, use Sign instead.
func RestoreEDDSASignature(sign []byte, signer hotstuff.ID) *EDDSASignature {
	return &EDDSASignature{signer, sign}
}

// Signer returns the ID of the replica that generated the signature.
func (sig EDDSASignature) Signer() hotstuff.ID {
	return sig.signer
}

// ToBytes returns a raw byte string representation of the signature.
func (sig EDDSASignature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.sign...)
	return b
}

// EDDSA implements the ed25519 curve signature.
type EDDSA struct {
	config *core.RuntimeConfig
}

// NewEDDSA returns a new instance of the EDDSA crypto implementation.
func NewEDDSA(config *core.RuntimeConfig) *EDDSA {
	return &EDDSA{
		config: config,
	}
}

func (ed *EDDSA) privateKey() ed25519.PrivateKey {
	return ed.config.PrivateKey().(ed25519.PrivateKey)
}

// Sign creates a cryptographic signature of the given message.
func (ed *EDDSA) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	sign := ed25519.Sign(ed.privateKey(), message)
	eddsaSign := &EDDSASignature{signer: ed.config.ID(), sign: sign}
	return Multi[*EDDSASignature]{ed.config.ID(): eddsaSign}, nil
}

// Combine combines multiple signatures into a single signature.
func (ed *EDDSA) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	if len(signatures) < 2 {
		return nil, ErrCombineMultiple
	}

	ts := make(Multi[*EDDSASignature])
	for _, sig1 := range signatures {
		if sig2, ok := sig1.(Multi[*EDDSASignature]); ok {
			for id, s := range sig2 {
				if _, duplicate := ts[id]; duplicate {
					return nil, ErrCombineOverlap
				}
				ts[id] = s
			}
		} else {
			return nil, fmt.Errorf("eddsa: cannot combine signature of incompatible type %T (expected %T)", sig1, sig2)
		}
	}
	return ts, nil
}

// Verify verifies the given quorum signature against the message.
func (ed *EDDSA) Verify(signature hotstuff.QuorumSignature, message []byte) error {
	s, ok := signature.(Multi[*EDDSASignature])
	if !ok {
		return fmt.Errorf("eddsa: cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	n := signature.Participants().Len()
	if n == 0 {
		return fmt.Errorf("eddsa: failed to verify: no participants")
	}

	results := make(chan error, n)
	for _, sig := range s {
		go func(sig *EDDSASignature, msg []byte) {
			results <- ed.verifySingle(sig, msg)
		}(sig, message)
	}
	var err error
	for range s {
		err = errors.Join(<-results)
	}
	if err != nil {
		return err
	}
	return nil
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (ed *EDDSA) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error {
	s, ok := signature.(Multi[*EDDSASignature])
	if !ok {
		return fmt.Errorf("eddsa: cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	n := signature.Participants().Len()
	if n == 0 {
		return fmt.Errorf("eddsa: failed to verify batch: no participants")
	}

	results := make(chan error, n)
	set := make(map[hotstuff.Hash]struct{})
	for id, sig := range s {
		message, ok := batch[id]
		if !ok {
			return fmt.Errorf("eddsa: message not found")
		}
		hash := sha256.Sum256(message)
		set[hash] = struct{}{}
		go func(sig *EDDSASignature, msg []byte) {
			results <- ed.verifySingle(sig, msg)
		}(sig, message)
	}
	var err error
	for range s {
		err = errors.Join(<-results)
	}
	if err != nil {
		return err
	}
	// valid if all partial signatures are valid and there are no duplicate messages
	if len(set) != len(batch) {
		return fmt.Errorf("eddsa: invalid signature")
	}
	return nil
}

func (ed *EDDSA) verifySingle(sig *EDDSASignature, message []byte) error {
	replica, ok := ed.config.ReplicaInfo(sig.Signer())
	if !ok {
		return fmt.Errorf("eddsa: failed to verify signature from replica %d: unknown replica", sig.Signer())
	}
	pk := replica.PubKey.(ed25519.PublicKey)
	if !ed25519.Verify(pk, message, sig.sign) {
		return fmt.Errorf("eddsa: failed to verify signature from replica %d", sig.Signer())
	}
	return nil
}

var (
	_ hotstuff.QuorumSignature = (*Multi[*EDDSASignature])(nil)
	_ hotstuff.IDSet           = (*Multi[*EDDSASignature])(nil)
	_ Signature                = (*EDDSASignature)(nil)
	_ Base                     = (*EDDSA)(nil)
)
