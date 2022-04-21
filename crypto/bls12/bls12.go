// Package bls12 implements the crypto primitives used by HotStuff using curve BLS12-381.
package bls12

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"

	bls12 "github.com/kilic/bls12-381"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/modules"
	"go.uber.org/multierr"
)

func init() {
	modules.RegisterModule("bls12", New)
}

const (
	// PrivateKeyFileType is the PEM type for a private key.
	PrivateKeyFileType = "BLS12-381 PRIVATE KEY"

	// PublicKeyFileType is the PEM type for a public key.
	PublicKeyFileType = "BLS12-381 PUBLIC KEY"
)

var domain = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

// the order r of G1
var curveOrder, _ = new(big.Int).SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001", 16)

// PublicKey is a bls12-381 public key.
type PublicKey struct {
	p *bls12.PointG1
}

// ToBytes marshals the public key to a byte slice.
func (pub PublicKey) ToBytes() []byte {
	return bls12.NewG1().ToCompressed(pub.p)
}

// FromBytes unmarshals the public key from a byte slice.
func (pub *PublicKey) FromBytes(b []byte) (err error) {
	pub.p, err = bls12.NewG1().FromCompressed(b)
	if err != nil {
		return fmt.Errorf("bls12: failed to decompress public key: %w", err)
	}
	return nil
}

// PrivateKey is a bls12-381 private key.
type PrivateKey struct {
	p *big.Int
}

// ToBytes marshals the private key to a byte slice.
func (priv PrivateKey) ToBytes() []byte {
	return priv.p.Bytes()
}

// FromBytes unmarshals the private key from a byte slice.
func (priv *PrivateKey) FromBytes(b []byte) {
	priv.p = new(big.Int)
	priv.p.SetBytes(b)
}

// GeneratePrivateKey generates a new private key.
func GeneratePrivateKey() (*PrivateKey, error) {
	// the private key is uniformly random integer such that 0 <= pk < r
	pk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to generate private key: %w", err)
	}
	return &PrivateKey{
		p: pk,
	}, nil
}

// Public returns the public key associated with this private key.
func (priv *PrivateKey) Public() consensus.PublicKey {
	p := &bls12.PointG1{}
	// The public key is the secret key multiplied by the generator G1
	return &PublicKey{p: bls12.NewG1().MulScalarBig(p, &bls12.G1One, priv.p)}
}

// Signature is a bls12-381 signature.
type Signature struct {
	signer hotstuff.ID
	s      *bls12.PointG2
}

// ToBytes returns the object as bytes.
func (s *Signature) ToBytes() []byte {
	var idBytes [4]byte
	binary.LittleEndian.PutUint32(idBytes[:], uint32(s.signer))
	// not sure if it is better to use compressed or uncompressed here.
	return append(idBytes[:], bls12.NewG2().ToCompressed(s.s)...)
}

// FromBytes unmarshals a signature from a byte slice.
func (s *Signature) FromBytes(b []byte) (err error) {
	s.signer = hotstuff.ID(binary.LittleEndian.Uint32(b))
	s.s, err = bls12.NewG2().FromCompressed(b[4:])
	if err != nil {
		return fmt.Errorf("bls12: failed to decompress signature: %w", err)
	}
	return nil
}

// Signer returns the ID of the replica that generated the signature.
func (s *Signature) Signer() hotstuff.ID {
	return s.signer
}

// AggregateSignature is a bls12-381 aggregate signature. The participants map contains the IDs of the replicas who
// participated in the creation of the signature. This allows us to build an aggregated public key to verify the
// signature.
type AggregateSignature struct {
	sig          bls12.PointG2
	participants crypto.Bitfield // The ids of the replicas who submitted signatures.
}

// RestoreAggregateSignature restores an existing aggregate signature. It should not be used to create new aggregate
// signatures. Use CreateThresholdSignature instead.
func RestoreAggregateSignature(sig []byte, participants crypto.Bitfield) (s *AggregateSignature, err error) {
	p, err := bls12.NewG2().FromCompressed(sig)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to restore aggregate signature: %w", err)
	}
	return &AggregateSignature{
		sig:          *p,
		participants: participants,
	}, nil
}

// ToBytes returns a byte representation of the aggregate signature.
func (agg *AggregateSignature) ToBytes() []byte {
	if agg == nil {
		return nil
	}
	b := bls12.NewG2().ToCompressed(&agg.sig)
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (agg AggregateSignature) Participants() consensus.IDSet {
	return &agg.participants
}

// Bitfield returns the bitmask.
func (agg AggregateSignature) Bitfield() crypto.Bitfield {
	return agg.participants
}

// AddSignatures adds additional signatures to the aggregate.
func (agg *AggregateSignature) AddSignatures(signatures map[hotstuff.ID]*Signature) {
	g2 := bls12.NewG2()
	for id, s := range signatures {
		g2.Add(&agg.sig, &agg.sig, s.s)
		agg.participants.Add(id)
	}
}

// bls12Crypto is a Signer/Verifier implementation that uses bls12-381 aggregate signatures.
type bls12Crypto struct {
	mods *consensus.Modules
}

// New returns a new bls12-381 signer and verifier.
func New() consensus.CryptoImpl {
	bc := &bls12Crypto{}
	return bc
}

func (bc *bls12Crypto) getPrivateKey() *PrivateKey {
	pk := bc.mods.PrivateKey()
	return pk.(*PrivateKey)
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (bc *bls12Crypto) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	bc.mods = mods
}

// Sign signs a hash.
func (bc *bls12Crypto) Sign(hash consensus.Hash) (sig consensus.Signature, err error) {
	p, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: hash to curve failed: %w", err)
	}
	pk := bc.getPrivateKey()
	bls12.NewG2().MulScalarBig(p, p, pk.p)
	return &Signature{signer: bc.mods.ID(), s: p}, nil
}

// AggregateSignatures aggregates the signatures to form a single aggregated signature.
func AggregateSignatures(signatures map[hotstuff.ID]*Signature) *AggregateSignature {
	if len(signatures) == 0 {
		return nil
	}
	g2 := bls12.NewG2()
	sig := bls12.PointG2{}
	var participants crypto.Bitfield
	for id, s := range signatures {
		g2.Add(&sig, &sig, s.s)
		participants.Add(id)
	}
	return &AggregateSignature{sig: sig, participants: participants}
}

// Verify verifies a signature given a hash.
func (bc *bls12Crypto) Verify(sig consensus.Signature, hash consensus.Hash) bool {
	s := sig.(*Signature)
	replica, ok := bc.mods.Configuration().Replica(sig.Signer())
	if !ok {
		bc.mods.Logger().Infof("bls12Crypto: got signature from replica whose ID (%d) was not in the config", sig.Signer())
	}
	pk := replica.PublicKey().(*PublicKey)
	p, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, s.s)
	engine.AddPair(pk.p, p)
	return engine.Result().IsOne()
}

// VerifyAggregateSignature verifies an aggregated signature.
// It does not check whether the aggregated signature contains a quorum of signatures.
func (bc *bls12Crypto) VerifyAggregateSignature(agg consensus.ThresholdSignature, hash consensus.Hash) bool {
	sig, ok := agg.(*AggregateSignature)
	if !ok {
		return false
	}
	pubKeys := make([]*PublicKey, 0)
	sig.participants.ForEach(func(id hotstuff.ID) {
		replica, ok := bc.mods.Configuration().Replica(id)
		if !ok {
			return
		}
		pubKeys = append(pubKeys, replica.PublicKey().(*PublicKey))
	})
	ps, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		bc.mods.Logger().Error(err)
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, &sig.sig)
	for _, pub := range pubKeys {
		engine.AddPair(pub.p, ps)
	}
	return engine.Result().IsOne()
}

// TODO: I'm not sure to what extent we are vulnerable to a rogue public key attack here.
// As far as I can tell, this is not a problem right now because we do not yet support reconfiguration,
// and all public keys are known by all replicas.

// VerifyThresholdSignature verifies a threshold signature.
func (bc *bls12Crypto) VerifyThresholdSignature(signature consensus.ThresholdSignature, hash consensus.Hash) bool {
	sig, ok := signature.(*AggregateSignature)
	if !ok {
		return false
	}
	pubKeys := make([]*PublicKey, 0)
	sig.participants.ForEach(func(id hotstuff.ID) {
		replica, ok := bc.mods.Configuration().Replica(id)
		if !ok {
			return
		}
		pubKeys = append(pubKeys, replica.PublicKey().(*PublicKey))
	})
	ps, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		bc.mods.Logger().Error(err)
		return false
	}
	if len(pubKeys) < bc.mods.Configuration().QuorumSize() {
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, &sig.sig)
	for _, pub := range pubKeys {
		engine.AddPair(pub.p, ps)
	}
	return engine.Result().IsOne()
}

// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
func (bc *bls12Crypto) VerifyThresholdSignatureForMessageSet(signature consensus.ThresholdSignature, hashes map[hotstuff.ID]consensus.Hash) bool {
	sig, ok := signature.(*AggregateSignature)
	if !ok {
		return false
	}
	hashSet := make(map[consensus.Hash]struct{})
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, &sig.sig)
	for id, hash := range hashes {
		if _, ok := hashSet[hash]; ok {
			continue
		}
		hashSet[hash] = struct{}{}
		replica, ok := bc.mods.Configuration().Replica(id)
		if !ok {
			return false
		}
		pk, ok := replica.PublicKey().(*PublicKey)
		if !ok {
			return false
		}
		p2, err := bls12.NewG2().HashToCurve(hash[:], domain)
		if err != nil {
			return false
		}
		engine.AddPair(pk.p, p2)
	}
	if !engine.Result().IsOne() {
		return false
	}
	// if we managed to verify the aggregate signature, we just need to make sure that the number of verified signatures
	// is a quorum.
	return len(hashSet) >= bc.mods.Configuration().QuorumSize()
}

// TODO: should we check each signature's validity before aggregating?

// CreateThresholdSignature creates a threshold signature from the given partial signatures.
func (bc *bls12Crypto) CreateThresholdSignature(partialSignatures []consensus.Signature, _ consensus.Hash) (_ consensus.ThresholdSignature, err error) {
	if len(partialSignatures) < bc.mods.Configuration().QuorumSize() {
		return nil, crypto.ErrNotAQuorum
	}
	sigs := make(map[hotstuff.ID]*Signature, len(partialSignatures))
	for _, sig := range partialSignatures {
		if _, ok := sigs[sig.Signer()]; ok {
			err = multierr.Append(err, crypto.ErrPartialDuplicate)
			continue
		}
		s, ok := sig.(*Signature)
		if !ok {
			err = multierr.Append(err, fmt.Errorf("%w: %T", crypto.ErrWrongType, s))
			continue
		}
		sigs[sig.Signer()] = s
	}
	if len(sigs) < bc.mods.Configuration().QuorumSize() {
		return nil, multierr.Combine(crypto.ErrNotAQuorum, err)
	}
	return AggregateSignatures(sigs), nil
}

// CreateThresholdSignatureForMessageSet creates a threshold signature where each partial signature has signed a
// different message hash.
func (bc *bls12Crypto) CreateThresholdSignatureForMessageSet(partialSignatures []consensus.Signature, hashes map[hotstuff.ID]consensus.Hash) (consensus.ThresholdSignature, error) {
	// Don't care about the hashes for signature aggregation.
	return bc.CreateThresholdSignature(partialSignatures, consensus.Hash{})
}

// Combine combines multiple signatures into a single threshold signature.
// Arguments can be singular signatures or threshold signatures.
//
// As opposed to the CreateThresholdSignature methods,
// this method does not check whether the resulting
// signature meets the quorum size.
func (bc *bls12Crypto) Combine(signatures ...interface{}) consensus.ThresholdSignature {
	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	var participants crypto.Bitfield
	for _, sig := range signatures {
		switch sig := sig.(type) {
		case *Signature:
			participants.Add(sig.signer)
			g2.Add(&agg, &agg, sig.s)
		case *AggregateSignature:
			sig.participants.ForEach(participants.Add)
			g2.Add(&agg, &agg, &sig.sig)
		}
	}
	return &AggregateSignature{sig: agg, participants: participants}
}
