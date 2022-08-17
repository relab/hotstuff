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
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/msg"
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
// //type Signature struct {
// 	r, s   *big.Int
// 	signer hotstuff.ID
// }

// // RestoreSignature restores an existing signature. It should not be used to create new signatures, use Sign instead.
// func RestoreSignature(r, s *big.Int, signer hotstuff.ID) *hotstuffpb.ECDSASignature {
// 	return &hotstuffpb.ECDSASignature{r, s, signer}
// }

// Signer returns the ID of the replica that generated the signature.
//func (sig Signature) Signer() hotstuff.ID {
//	return sig.signer
//}

// R returns the r value of the signature
// func (sig Signature) R() *big.Int {
// 	return sig.r
// }

// // S returns the s value of the signature
// func (sig Signature) S() *big.Int {
// 	return sig.s
// }

// ToBytes returns a raw byte string representation of the signature

// MultiSignature is a set of (partial) signatures.
// type MultiSignature map[hotstuff.ID]*hotstuffpb.ECDSASignature

// // RestoreMultiSignature should only be used to restore an existing threshold signature from a set of signatures.
// func RestoreMultiSignature(signatures []*hotstuffpb.ECDSASignature) MultiSignature {
// 	sig := make(MultiSignature, len(signatures))
// 	for _, s := range signatures {
// 		sig[hotstuff.ID(s.GetSigner())] = s
// 	}
// 	return sig
// }

// // ToBytes returns the object as bytes.
// func (sig MultiSignature) ToBytes() []byte {
// 	var b []byte
// 	// sort by ID to make it deterministic
// 	order := make([]hotstuff.ID, 0, len(sig))
// 	for _, signature := range sig {
// 		order = append(order, hotstuff.ID(signature.Signer))
// 	}
// 	slices.Sort(order)
// 	for _, id := range order {
// 		b = append(b, sig[id].ToBytes()...)
// 	}
// 	return b
// }

// // Participants returns the IDs of replicas who participated in the threshold signature.
// func (sig MultiSignature) Participants() msg.IDSet {
// 	return sig
// }

// // Add adds an ID to the set.
// func (sig MultiSignature) Add(id hotstuff.ID) {
// 	panic("not implemented")
// }

// // Contains returns true if the set contains the ID.
// func (sig MultiSignature) Contains(id hotstuff.ID) bool {
// 	_, ok := sig[id]
// 	return ok
// }

// // ForEach calls f for each ID in the set.
// func (sig MultiSignature) ForEach(f func(hotstuff.ID)) {
// 	for id := range sig {
// 		f(id)
// 	}
// }

// // RangeWhile calls f for each ID in the set until f returns false.
// func (sig MultiSignature) RangeWhile(f func(hotstuff.ID) bool) {
// 	for id := range sig {
// 		if !f(id) {
// 			break
// 		}
// 	}
// }

// // Len returns the number of entries in the set.
// func (sig MultiSignature) Len() int {
// 	return len(sig)
// }

// func (sig MultiSignature) String() string {
// 	return msg.IDSetToString(sig)
// }

// var _ msg.QuorumSignature = (*MultiSignature)(nil)
// var _ msg.IDSet = (*MultiSignature)(nil)

type ecdsaBase struct {
	mods *modules.ConsensusCore
}

// New returns a new instance of the ECDSA CryptoBase implementation.
func New() modules.CryptoBase {
	return &ecdsaBase{}
}

func (ec *ecdsaBase) getPrivateKey() *ecdsa.PrivateKey {
	pk := ec.mods.PrivateKey()
	return pk.(*ecdsa.PrivateKey)
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (ec *ecdsaBase) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	ec.mods = mods
}

// Sign creates a cryptographic signature of the given message.
func (ec *ecdsaBase) Sign(message []byte) (signature *msg.Signature, err error) {
	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign failed: %w", err)
	}
	return &msg.Signature{
		ID: uint32(ec.mods.ID()),
		Sig: &msg.Signature_ECDSASig{
			ECDSASig: &msg.ECDSASignature{
				Signer: uint32(ec.mods.ID()),
				R:      r.Bytes(),
				S:      s.Bytes(),
			},
		},
	}, nil
}

// Combine combines multiple signatures into a single signature.
func (ec *ecdsaBase) Combine(signatures ...*msg.ThresholdSignature) (*msg.ThresholdSignature, error) {
	if len(signatures) < 2 {
		return nil, crypto.ErrCombineMultiple
	}

	ts := make([]*msg.ECDSASignature, 0)
	signers := make(map[uint32]bool)
	for _, sig1 := range signatures {
		if sig2, ok := sig1.AggSig.(*msg.ThresholdSignature_ECDSASigs); ok {
			for _, s := range sig2.ECDSASigs.Sigs {
				signer := s.GetSigner()
				if _, ok := signers[signer]; ok {
					return nil, crypto.ErrCombineOverlap
				}
				signers[signer] = true
				ts = append(ts, s)
			}
		} else {
			ec.mods.Logger().Panicf("cannot combine signature of incompatible type %T (expected %T)", sig1, sig2)
		}
	}
	return &msg.ThresholdSignature{
		AggSig: &msg.ThresholdSignature_ECDSASigs{
			ECDSASigs: &msg.ECDSAThresholdSignature{
				Sigs: ts,
			},
		}}, nil
}

// Verify verifies the given quorum signature against the message.
func (ec *ecdsaBase) Verify(signature *msg.ThresholdSignature, message []byte) bool {
	s, ok := signature.AggSig.(*msg.ThresholdSignature_ECDSASigs)
	if !ok {
		ec.mods.Logger().Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}

	sigs := s.ECDSASigs.Sigs
	n := len(sigs)
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	hash := sha256.Sum256(message)

	for _, sig := range sigs {
		go func(sig *msg.ECDSASignature, hash msg.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
	}

	valid := true
	for range sigs {
		if !<-results {
			valid = false
		}
	}

	return valid
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (ec *ecdsaBase) BatchVerify(signature *msg.ThresholdSignature, batch map[hotstuff.ID][]byte) bool {
	s, ok := signature.AggSig.(*msg.ThresholdSignature_ECDSASigs)
	if !ok {
		ec.mods.Logger().Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}
	sigs := s.ECDSASigs.Sigs
	n := len(sigs)
	if n == 0 {
		return false
	}

	results := make(chan bool, n)
	set := make(map[msg.Hash]struct{})
	for _, sig := range sigs {
		message, ok := batch[hotstuff.ID(sig.GetSigner())]
		if !ok {
			return false
		}
		hash := sha256.Sum256(message)
		set[hash] = struct{}{}
		go func(sig *msg.ECDSASignature, hash msg.Hash) {
			results <- ec.verifySingle(sig, hash)
		}(sig, hash)
	}

	valid := true
	for range sigs {
		if !<-results {
			valid = false
		}
	}

	// valid if all partial signatures are valid and there are no duplicate messages
	return valid && len(set) == len(batch)
}

func (ec *ecdsaBase) verifySingle(sig *msg.ECDSASignature, hash msg.Hash) bool {
	replica, ok := ec.mods.Configuration().Replica(hotstuff.ID(sig.GetSigner()))
	if !ok {
		ec.mods.Logger().Warnf("ecdsaBase: got signature from replica whose ID (%d) was not in the config.", sig.GetSigner())
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	r := new(big.Int)
	r.SetBytes(sig.GetR())
	s := new(big.Int)
	s.SetBytes(sig.GetS())
	return ecdsa.Verify(pk, hash[:], r, s)
}
