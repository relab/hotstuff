// Package bls12 implements the crypto primitives used by HotStuff using curve BLS12-381.
package bls12

import (
	"crypto/rand"
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

	popMetadataKey = "bls12-pop"
)

var (
	domain    = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")
	domainPOP = []byte("BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

	// the order r of G1
	curveOrder, _ = new(big.Int).SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001", 16)
)

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
func (bls *bls12Base) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	bls.mods = mods

	pop := bls.popProve()
	b := bls12.NewG2().ToCompressed(pop)
	opts.SetConnectionMetadata(popMetadataKey, string(b))
}

func (bls *bls12Base) getPrivateKey() *PrivateKey {
	pk := bls.mods.PrivateKey()
	return pk.(*PrivateKey)
}

func (bls *bls12Base) subgroupCheck(point *bls12.PointG2) bool {
	var p bls12.PointG2
	g2 := bls12.NewG2()
	g2.MulScalarBig(&p, point, curveOrder)
	return g2.IsZero(&p)
}

func (bls *bls12Base) coreSign(message []byte, domainTag []byte) (*bls12.PointG2, error) {
	pk := bls.getPrivateKey()
	g2 := bls12.NewG2()
	point, err := g2.HashToCurve(message, domainTag)
	if err != nil {
		return nil, err
	}
	// multiply the point by the secret key, storing the result in the same point variable
	g2.MulScalarBig(point, point, pk.p)
	return point, nil
}

func (bls *bls12Base) coreVerify(pubKey *PublicKey, message []byte, signature *bls12.PointG2, domainTag []byte) bool {
	if !bls.subgroupCheck(signature) {
		return false
	}
	g2 := bls12.NewG2()
	messagePoint, err := g2.HashToCurve(message, domainTag)
	if err != nil {
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, signature)
	engine.AddPair(pubKey.p, messagePoint)
	return engine.Result().IsOne()
}

func (bls *bls12Base) popProve() *bls12.PointG2 {
	pubKey := bls.getPrivateKey().Public().(*PublicKey)
	proof, err := bls.coreSign(pubKey.ToBytes(), domainPOP)
	if err != nil {
		bls.mods.Logger().Panicf("failed to generate proof of possession: %v", err)
	}
	return proof
}

func (bls *bls12Base) popVerify(pubKey *PublicKey, proof *bls12.PointG2) bool {
	return bls.coreVerify(pubKey, pubKey.ToBytes(), proof, domainPOP)
}

func (bls *bls12Base) checkPop(replica consensus.Replica) bool {
	b, ok := replica.Metadata()[popMetadataKey]
	if !ok {
		return false
	}
	p, err := bls12.NewG2().FromCompressed([]byte(b))
	if err != nil {
		return false
	}
	return bls.popVerify(replica.PublicKey().(*PublicKey), p)
}

// Sign creates a cryptographic signature of the given hash.
func (bls *bls12Base) Sign(hash consensus.Hash) (signature consensus.QuorumSignature, err error) {
	// TODO: consider passing message to this function instead of hash
	p, err := bls.coreSign(hash[:], domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: coreSign failed: %w", err)
	}
	bf := crypto.Bitfield{}
	bf.Add(bls.mods.ID())
	return &AggregateSignature{sig: *p, participants: bf}, nil
}

// Verify verifies the given cryptographic signature according to the specified options.
// NOTE: One of either VerifyHash or VerifyHashes MUST be specified,
// otherwise this function will have nothing to verify the signature against.
func (bls *bls12Base) Verify(signature consensus.QuorumSignature, options ...consensus.VerifyOption) bool {
	sig, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify signature of incompatible type %T (expected %T)", signature, sig))
	}

	var opts consensus.VerifyOptions
	for _, opt := range options {
		opt(&opts)
	}

	if !opts.UseHashMap && opts.Hash == nil {
		panic("no hash(es) to verify the signature against: you must specify one of the VerifyHash or VerifyHashes options")
	}

	if signature.Participants().Len() < opts.Threshold {
		return false
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

		if !bls.checkPop(replica) {
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

	return engine.Result().IsOne()
}

// Combine combines multiple signatures into a single signature.
func (bls *bls12Base) Combine(signatures ...consensus.QuorumSignature) (consensus.QuorumSignature, error) {
	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	var participants crypto.Bitfield
	for _, sig := range signatures {
		if sig, ok := sig.(*AggregateSignature); ok {
			sig.participants.ForEach(participants.Add)
			g2.Add(&agg, &agg, &sig.sig)
		}
	}
	return &AggregateSignature{sig: agg, participants: participants}, nil
}
