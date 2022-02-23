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

type bls12Base struct {
	mods *consensus.Modules
}

// New returns a new instance of the BLS12 CryptoBase implementation.
func New() consensus.CryptoBase {
	return &bls12Base{}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (bls *bls12Base) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	bls.mods = mods
}

func (bls *bls12Base) getPrivateKey() *PrivateKey {
	pk := bls.mods.PrivateKey()
	return pk.(*PrivateKey)
}

// Sign creates a cryptographic signature of the given hash.
func (bls *bls12Base) Sign(hash consensus.Hash) (signature consensus.QuorumSignature, err error) {
	p, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: hash to curve failed: %w", err)
	}
	pk := bls.getPrivateKey()
	bls12.NewG2().MulScalarBig(p, p, pk.p)
	var bf crypto.Bitfield
	bf.Add(bls.mods.ID())
	return &AggregateSignature{sig: *p, participants: bf}, nil
}

// Verify verifies the given cryptographic signature according to the specified options.
// NOTE: One of either VerifyHash or VerifyHashes MUST be specified,
// otherwise this function will have nothing to verify the signature against.
func (bls *bls12Base) Verify(signature consensus.QuorumSignature, options ...consensus.VerifyOption) bool {
	sig, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify signature of incompatible type %T (expected %T", signature, sig))
	}

	var opts consensus.VerifyOptions
	for _, opt := range options {
		opt(&opts)
	}

	if !opts.UseHashMap && opts.Hash == nil {
		panic(fmt.Sprintf("no hash(es) to verify the signature against: you must specify one of the VerifyHash or VerifyHashes options"))
	}

	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, &sig.sig)

	var (
		ps  *bls12.PointG2
		err error
	)
	if !opts.UseHashMap {
		ps, err = bls12.NewG2().HashToCurve((*opts.Hash)[:], domain)
		if err != nil {
			bls.mods.Logger().Errorf("HashToCurve failed: %v", err)
			return false
		}
	}

	valid := true

	sig.participants.RangeWhile(func(i hotstuff.ID) bool {
		replica, ok := bls.mods.Configuration().Replica(i)
		if !ok {
			valid = false
			return false
		}

		pk := replica.PublicKey().(*PublicKey)

		var (
			p2  *bls12.PointG2
			err error
		)
		if opts.UseHashMap {
			hash, ok := opts.HashMap[i]
			if !ok {
				valid = false
				return false
			}

			p2, err = bls12.NewG2().HashToCurve(hash[:], domain)
			if err != nil {
				bls.mods.Logger().Errorf("HashToCurve failed: %v", err)
				valid = false
				return false
			}
		} else {
			p2 = ps
		}

		engine.AddPair(pk.p, p2)

		return true
	})

	if !valid {
		return false
	}

	if !engine.Result().IsOne() {
		return false
	}

	return signature.Participants().Len() >= opts.Threshold
}

// Combine combines multiple signatures into a single signature.
func (bls *bls12Base) Combine(signatures ...consensus.QuorumSignature) consensus.QuorumSignature {
	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	var participants crypto.Bitfield
	for _, sig := range signatures {
		if sig, ok := sig.(*AggregateSignature); ok {
			sig.participants.ForEach(participants.Add)
			g2.Add(&agg, &agg, &sig.sig)
		}
	}
	return &AggregateSignature{sig: agg, participants: participants}
}
