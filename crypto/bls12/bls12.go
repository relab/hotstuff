// Package bls12 implements the crypto primitives used by HotStuff using curve BLS12-381.
package bls12

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"sync"

	bls12 "github.com/kilic/bls12-381"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/logging"
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

	popMetadataKey = "bls12-pop-bin"
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
func (priv *PrivateKey) Public() hotstuff.PublicKey {
	p := &bls12.PointG1{}
	// The public key is the secret key multiplied by the generator G1
	return &PublicKey{p: bls12.NewG1().MulScalarBig(p, &bls12.G1One, priv.p)}
}

// AggregateSignature is a bls12-381 aggregate signature. The participants field contains the IDs of the replicas that
// participated in signature creation. This allows us to build an aggregated public key to verify the signature.
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
func (agg AggregateSignature) Participants() hotstuff.IDSet {
	return &agg.participants
}

// Bitfield returns the bitmask.
func (agg AggregateSignature) Bitfield() crypto.Bitfield {
	return agg.participants
}

func firstParticipant(participants hotstuff.IDSet) hotstuff.ID {
	id := hotstuff.ID(0)
	participants.RangeWhile(func(i hotstuff.ID) bool {
		id = i
		return false
	})
	return id
}

type bls12Base struct {
	configuration modules.Configuration
	logger        logging.Logger
	opts          *modules.Options

	mut sync.RWMutex
	// popCache caches the proof-of-possession results of popVerify for each public key.
	popCache map[string]bool
}

// New returns a new instance of the BLS12 CryptoBase implementation.
func New() modules.CryptoBase {
	return &bls12Base{
		popCache: make(map[string]bool),
	}
}

// InitModule gives the module a reference to the Core object.
// It also allows the module to set module options using the OptionsBuilder.
func (bls *bls12Base) InitModule(mods *modules.Core) {
	mods.Get(
		&bls.configuration,
		&bls.logger,
		&bls.opts,
	)

	pop := bls.popProve()
	b := bls12.NewG2().ToCompressed(pop)
	bls.opts.SetConnectionMetadata(popMetadataKey, string(b))
}

func (bls *bls12Base) privateKey() *PrivateKey {
	return bls.opts.PrivateKey().(*PrivateKey)
}

func (bls *bls12Base) publicKey(id hotstuff.ID) (pubKey *PublicKey, ok bool) {
	if replica, ok := bls.configuration.Replica(id); ok {
		if replica.ID() != bls.opts.ID() && !bls.checkPop(replica) {
			bls.logger.Warnf("Invalid POP for replica %d", id)
			return nil, false
		}
		if pubKey, ok = replica.PublicKey().(*PublicKey); ok {
			return pubKey, true
		}
		bls.logger.Errorf("Unsupported public key type: %T", replica.PublicKey())
	}
	return nil, false
}

func (bls *bls12Base) subgroupCheck(point *bls12.PointG2) bool {
	var p bls12.PointG2
	g2 := bls12.NewG2()
	g2.MulScalarBig(&p, point, curveOrder)
	return g2.IsZero(&p)
}

func (bls *bls12Base) coreSign(message []byte, domainTag []byte) (*bls12.PointG2, error) {
	pk := bls.privateKey()
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
	pubKey := bls.privateKey().Public().(*PublicKey)
	proof, err := bls.coreSign(pubKey.ToBytes(), domainPOP)
	if err != nil {
		bls.logger.Panicf("Failed to generate proof-of-possession: %v", err)
	}
	return proof
}

func (bls *bls12Base) popVerify(pubKey *PublicKey, proof *bls12.PointG2) bool {
	return bls.coreVerify(pubKey, pubKey.ToBytes(), proof, domainPOP)
}

func (bls *bls12Base) checkPop(replica modules.Replica) (valid bool) {
	defer func() {
		if !valid {
			bls.logger.Warnf("Invalid proof-of-possession for replica %d", replica.ID())
		}
	}()

	popBytes, ok := replica.Metadata()[popMetadataKey]
	if !ok {
		bls.logger.Warnf("Missing proof-of-possession for replica: %d", replica.ID())
		return false
	}

	var key strings.Builder
	key.WriteString(popBytes)
	_, _ = key.Write(replica.PublicKey().(*PublicKey).ToBytes())

	bls.mut.RLock()
	valid, ok = bls.popCache[key.String()]
	bls.mut.RUnlock()
	if ok {
		return valid
	}

	proof, err := bls12.NewG2().FromCompressed([]byte(popBytes))
	if err != nil {
		return false
	}

	valid = bls.popVerify(replica.PublicKey().(*PublicKey), proof)

	bls.mut.Lock()
	bls.popCache[key.String()] = valid
	bls.mut.Unlock()

	return valid
}

func (bls *bls12Base) coreAggregateVerify(publicKeys []*PublicKey, messages [][]byte, signature *bls12.PointG2) bool {
	n := len(publicKeys)
	// validate input
	if n != len(messages) {
		return false
	}

	// precondition n >= 1
	if n < 1 {
		return false
	}

	if !bls.subgroupCheck(signature) {
		return false
	}

	engine := bls12.NewEngine()

	for i := 0; i < n; i++ {
		q, err := engine.G2.HashToCurve(messages[i], domain)
		if err != nil {
			return false
		}
		engine.AddPair(publicKeys[i].p, q)
	}

	engine.AddPairInv(&bls12.G1One, signature)
	return engine.Result().IsOne()
}

func (bls *bls12Base) aggregateVerify(publicKeys []*PublicKey, messages [][]byte, signature *bls12.PointG2) bool {
	set := make(map[string]struct{})
	for _, m := range messages {
		set[string(m)] = struct{}{}
	}
	return len(messages) == len(set) && bls.coreAggregateVerify(publicKeys, messages, signature)
}

func (bls *bls12Base) fastAggregateVerify(publicKeys []*PublicKey, message []byte, signature *bls12.PointG2) bool {
	engine := bls12.NewEngine()
	var aggregate bls12.PointG1
	for _, pk := range publicKeys {
		engine.G1.Add(&aggregate, &aggregate, pk.p)
	}
	return bls.coreVerify(&PublicKey{p: &aggregate}, message, signature, domain)
}

// Sign creates a cryptographic signature of the given messsage.
func (bls *bls12Base) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	p, err := bls.coreSign(message, domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: coreSign failed: %w", err)
	}
	bf := crypto.Bitfield{}
	bf.Add(bls.opts.ID())
	return &AggregateSignature{sig: *p, participants: bf}, nil
}

// Combine combines multiple signatures into a single signature.
func (bls *bls12Base) Combine(signatures ...hotstuff.QuorumSignature) (combined hotstuff.QuorumSignature, err error) {
	if len(signatures) < 2 {
		return nil, crypto.ErrCombineMultiple
	}

	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	var participants crypto.Bitfield
	for _, sig1 := range signatures {
		if sig2, ok := sig1.(*AggregateSignature); ok {
			sig2.participants.RangeWhile(func(id hotstuff.ID) bool {
				if participants.Contains(id) {
					err = crypto.ErrCombineOverlap
					return false
				}
				participants.Add(id)
				return true
			})
			if err != nil {
				return nil, err
			}
			g2.Add(&agg, &agg, &sig2.sig)
		} else {
			bls.logger.Panicf("cannot combine incompatible signature type %T (expected %T)", sig1, sig2)
		}
	}
	return &AggregateSignature{sig: agg, participants: participants}, nil
}

// Verify verifies the given quorum signature against the message.
func (bls *bls12Base) Verify(signature hotstuff.QuorumSignature, message []byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		bls.logger.Panicf("cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}

	n := s.Participants().Len()

	if n == 1 {
		id := firstParticipant(s.Participants())
		pk, ok := bls.publicKey(id)
		if !ok {
			bls.logger.Warnf("Missing public key for ID %d", id)
			return false
		}
		return bls.coreVerify(pk, message, &s.sig, domain)
	}

	// else if l > 1:
	pks := make([]*PublicKey, 0, n)
	s.Participants().RangeWhile(func(id hotstuff.ID) bool {
		pk, ok := bls.publicKey(id)
		if ok {
			pks = append(pks, pk)
			return true
		}
		bls.logger.Warnf("Missing public key for ID %d", id)
		return false
	})
	if len(pks) != n {
		return false
	}
	return bls.fastAggregateVerify(pks, message, &s.sig)
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (bls *bls12Base) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		bls.logger.Panicf("cannot verify incompatible signature type %T (expected %T)", signature, s)
	}

	if s.Participants().Len() != len(batch) {
		return false
	}

	pks := make([]*PublicKey, 0, len(batch))
	msgs := make([][]byte, 0, len(batch))

	for id, msg := range batch {
		msgs = append(msgs, msg)
		pk, ok := bls.publicKey(id)
		if !ok {
			bls.logger.Warnf("Missing public key for ID %d", id)
			return false
		}
		pks = append(pks, pk)
	}

	if len(batch) == 1 {
		return bls.coreVerify(pks[0], msgs[0], &s.sig, domain)
	}

	return bls.aggregateVerify(pks, msgs, &s.sig)
}
