package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const NameECDSA = "ecdsa"

const (
	// PrivateKeyFileType is the PEM type for a private key.
	ECDSAPrivateKeyFileType = "ECDSA PRIVATE KEY"
	// PublicKeyFileType is the PEM type for a public key.
	ECDSAPublicKeyFileType = "ECDSA PUBLIC KEY"
)

// ECDSASignature is an ECDSA signature.
type ECDSASignature struct {
	sig    []byte
	signer hotstuff.ID
}

// RestoreECDSASignature restores an existing signature.
// It should not be used to create new signatures, use Sign instead.
func RestoreECDSASignature(sig []byte, signer hotstuff.ID) *ECDSASignature {
	return &ECDSASignature{sig, signer}
}

// Signer returns the ID of the replica that generated the signature.
func (sig ECDSASignature) Signer() hotstuff.ID {
	return sig.signer
}

// ToBytes returns a raw byte string representation of the signature.
func (sig ECDSASignature) ToBytes() []byte {
	return sig.sig
}

// ECDSA implements the spec-k256 curve signature.
type ECDSA struct {
	config *core.RuntimeConfig
}

// NewECDSA returns a new instance of the ECDSA crypto implementation.
func NewECDSA(config *core.RuntimeConfig) *ECDSA {
	return &ECDSA{
		config: config,
	}
}

func (ec *ECDSA) privateKey() *ecdsa.PrivateKey {
	return ec.config.PrivateKey().(*ecdsa.PrivateKey)
}

// Sign creates a cryptographic signature of the given message.
func (ec *ECDSA) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	hash := sha256.Sum256(message)
	sig, err := ecdsa.SignASN1(rand.Reader, ec.privateKey(), hash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign failed: %w", err)
	}
	return Multi[*ECDSASignature]{ec.config.ID(): &ECDSASignature{
		sig:    sig,
		signer: ec.config.ID(),
	}}, nil
}

// Combine combines multiple signatures into a single signature.
func (ec *ECDSA) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	if len(signatures) < 2 {
		return nil, ErrCombineMultiple
	}

	ts := make(Multi[*ECDSASignature])
	for _, sig1 := range signatures {
		if sig2, ok := sig1.(Multi[*ECDSASignature]); ok {
			for id, s := range sig2 {
				if _, duplicate := ts[id]; duplicate {
					return nil, ErrCombineOverlap
				}
				ts[id] = s
			}
		} else {
			return nil, fmt.Errorf("ecdsa: cannot combine signature of incompatible type %T (expected %T)", sig1, sig2)
		}
	}
	return ts, nil
}

// Verify verifies the given quorum signature against the message.
func (ec *ECDSA) Verify(signature hotstuff.QuorumSignature, message []byte) error {
	s, ok := signature.(Multi[*ECDSASignature])
	if !ok {
		return fmt.Errorf("ecdsa: cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	n := signature.Participants().Len()
	if n == 0 {
		return fmt.Errorf("ecdsa: failed to verify: no participants")
	}

	results := make(chan error, n)
	hash := sha256.Sum256(message)
	for _, sig := range s {
		go func(sig *ECDSASignature, hash hotstuff.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
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
func (ec *ECDSA) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) (err error) {
	s, ok := signature.(Multi[*ECDSASignature])
	if !ok {
		return fmt.Errorf("ecdsa: cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	n := signature.Participants().Len()
	if n == 0 {
		return fmt.Errorf("ecdsa: failed to verify batch: no participants")
	}

	results := make(chan error, n)
	set := make(map[hotstuff.Hash]struct{})
	for id, sig := range s {
		message, ok := batch[id]
		if !ok {
			return fmt.Errorf("ecdsa: message not found")
		}
		hash := sha256.Sum256(message)
		set[hash] = struct{}{}
		go func(sig *ECDSASignature, hash hotstuff.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
	}
	for range s {
		err = errors.Join(<-results)
	}
	if err != nil {
		return err
	}
	// valid if all partial signatures are valid and there are no duplicate messages
	if len(set) != len(batch) {
		return fmt.Errorf("ecdsa: invalid signature")
	}
	return nil
}

func (ec *ECDSA) verifySingle(sig *ECDSASignature, hash hotstuff.Hash) error {
	replica, ok := ec.config.ReplicaInfo(sig.Signer())
	if !ok {
		return fmt.Errorf("ecdsa: failed to verify signature from replica %d: unknown replica", sig.Signer())
	}
	pk := replica.PubKey.(*ecdsa.PublicKey)
	if !ecdsa.VerifyASN1(pk, hash[:], sig.sig) {
		return fmt.Errorf("ecdsa: failed to verify signature from replica %d", sig.Signer())
	}
	return nil
}

var (
	_ hotstuff.QuorumSignature = (*Multi[*ECDSASignature])(nil)
	_ hotstuff.IDSet           = (*Multi[*ECDSASignature])(nil)
	_ Signature                = (*ECDSASignature)(nil)
	_ Base                     = (*ECDSA)(nil)
)
