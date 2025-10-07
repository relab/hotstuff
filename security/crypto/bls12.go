package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	bls12 "github.com/kilic/bls12-381"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const NameBLS12 = "bls12"

const (
	// PrivateKeyFileType is the PEM type for a private key.
	BLS12PrivateKeyFileType = "BLS12-381 PRIVATE KEY"

	// PublicKeyFileType is the PEM type for a public key.
	BLS12PublicKeyFileType = "BLS12-381 PUBLIC KEY"

	popMetadataKey = "bls12-pop-bin"
)

var (
	domain    = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")
	domainPOP = []byte("BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

	// the order r of G1
	curveOrder, _ = new(big.Int).SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001", 16)
)

// BLS12PublicKey is a bls12-381 public key.
type BLS12PublicKey struct {
	p *bls12.PointG1
}

// ToBytes marshals the public key to a byte slice.
func (pub BLS12PublicKey) ToBytes() []byte {
	return bls12.NewG1().ToCompressed(pub.p)
}

// FromBytes unmarshals the public key from a byte slice.
func (pub *BLS12PublicKey) FromBytes(b []byte) error {
	var err error
	pub.p, err = bls12.NewG1().FromCompressed(b)
	if err != nil {
		return fmt.Errorf("bls12: failed to decompress public key: %w", err)
	}
	return nil
}

// BLS12PrivateKey is a bls12-381 private key.
type BLS12PrivateKey struct {
	p *big.Int
}

// ToBytes marshals the private key to a byte slice.
func (priv BLS12PrivateKey) ToBytes() []byte {
	return priv.p.Bytes()
}

// FromBytes unmarshals the private key from a byte slice.
func (priv *BLS12PrivateKey) FromBytes(b []byte) {
	priv.p = new(big.Int)
	priv.p.SetBytes(b)
}

// GenerateBLS12PrivateKey generates a new private key.
func GenerateBLS12PrivateKey() (*BLS12PrivateKey, error) {
	// the private key is uniformly random integer such that 0 <= pk < r
	pk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to generate private key: %w", err)
	}
	return &BLS12PrivateKey{
		p: pk,
	}, nil
}

// Public returns the public key associated with this private key.
func (priv *BLS12PrivateKey) Public() hotstuff.PublicKey {
	p := &bls12.PointG1{}
	// The public key is the secret key multiplied by the generator G1
	return &BLS12PublicKey{p: bls12.NewG1().MulScalarBig(p, &bls12.G1One, priv.p)}
}

// BLS12AggregateSignature is a bls12-381 aggregate signature. The participants field contains the IDs of the replicas that
// participated in signature creation. This allows us to build an aggregated public key to verify the signature.
type BLS12AggregateSignature struct {
	sig          bls12.PointG2
	participants Bitfield // The ids of the replicas who submitted signatures.
}

// RestoreBLS12AggregateSignature restores an existing aggregate signature. It should not be used to create new aggregate
// signatures. Use CreateThresholdSignature instead.
func RestoreBLS12AggregateSignature(sig []byte, participants Bitfield) (s *BLS12AggregateSignature, err error) {
	p, err := bls12.NewG2().FromCompressed(sig)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to restore aggregate signature: %w", err)
	}
	return &BLS12AggregateSignature{
		sig:          *p,
		participants: participants,
	}, nil
}

// ToBytes returns a byte representation of the aggregate signature.
func (agg *BLS12AggregateSignature) ToBytes() []byte {
	if agg == nil {
		return nil
	}
	b := bls12.NewG2().ToCompressed(&agg.sig)
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (agg BLS12AggregateSignature) Participants() hotstuff.IDSet {
	return &agg.participants
}

// Bitfield returns the bitmask.
func (agg BLS12AggregateSignature) Bitfield() Bitfield {
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

// bls12Base implements the crypto primitives used by HotStuff using curve BLS12-381.
type bls12Base struct {
	config *core.RuntimeConfig

	mut sync.RWMutex
	// popCache caches the proof-of-possession results of popVerify for each public key.
	popCache map[string]bool
}

// NewBLS12 returns a new instance of the BLS12 crypto implementation.
func NewBLS12(config *core.RuntimeConfig) (Base, error) {
	bls := &bls12Base{
		config: config,

		popCache: make(map[string]bool),
	}

	pop, err := bls.popProve()
	if err != nil {
		return nil, err
	}
	b := bls12.NewG2().ToCompressed(pop)
	bls.config.AddConnectionMetadata(popMetadataKey, string(b))
	return bls, nil
}

func (bls *bls12Base) privateKey() *BLS12PrivateKey {
	return bls.config.PrivateKey().(*BLS12PrivateKey)
}

func (bls *bls12Base) publicKey(id hotstuff.ID) (pubKey *BLS12PublicKey, err error) {
	replica, ok := bls.config.ReplicaInfo(id)
	if !ok {
		return nil, fmt.Errorf("bls12: replica %d not found", id)
	}
	pubKey, ok = replica.PubKey.(*BLS12PublicKey)
	if !ok {
		return nil, fmt.Errorf("bls12: unsupported public key type: %T", replica.PubKey)
	}
	// not checking proof-of-possession for self.
	if id == bls.config.ID() {
		return pubKey, nil
	}
	if err := bls.checkPop(replica); err != nil {
		return nil, err
	}
	return pubKey, nil
}

func (bls *bls12Base) subgroupCheck(point *bls12.PointG2) error {
	var p bls12.PointG2
	g2 := bls12.NewG2()
	g2.MulScalarBig(&p, point, curveOrder)
	if !g2.IsZero(&p) {
		return fmt.Errorf("bls12: point is not part of the subgroup")
	}
	return nil
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

func (bls *bls12Base) coreVerify(pubKey *BLS12PublicKey, message []byte, signature *bls12.PointG2, domainTag []byte) error {
	if err := bls.subgroupCheck(signature); err != nil {
		return err
	}
	g2 := bls12.NewG2()
	messagePoint, err := g2.HashToCurve(message, domainTag)
	if err != nil {
		return err
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, signature)
	engine.AddPair(pubKey.p, messagePoint)
	if !engine.Result().IsOne() {
		return fmt.Errorf("bls12: failed to verify message")
	}
	return nil
}

func (bls *bls12Base) popProve() (*bls12.PointG2, error) {
	pubKey := bls.privateKey().Public().(*BLS12PublicKey)
	proof, err := bls.coreSign(pubKey.ToBytes(), domainPOP)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to generate proof-of-possession: %w", err)
	}
	return proof, nil
}

func (bls *bls12Base) popVerify(pubKey *BLS12PublicKey, proof *bls12.PointG2) error {
	return bls.coreVerify(pubKey, pubKey.ToBytes(), proof, domainPOP)
}

func (bls *bls12Base) checkPop(replica *hotstuff.ReplicaInfo) error {
	popBytes, ok := replica.Metadata[popMetadataKey]
	if !ok {
		return fmt.Errorf("bls12: missing proof-of-possession for replica: %d", replica.ID)
	}

	var key strings.Builder
	key.WriteString(popBytes)
	_, _ = key.Write(replica.PubKey.(*BLS12PublicKey).ToBytes())

	bls.mut.RLock()
	valid, ok := bls.popCache[key.String()]
	bls.mut.RUnlock()
	if ok && valid {
		return nil
	}

	proof, err := bls12.NewG2().FromCompressed([]byte(popBytes))
	if err != nil {
		return err
	}

	err = bls.popVerify(replica.PubKey.(*BLS12PublicKey), proof)
	valid = err == nil

	bls.mut.Lock()
	bls.popCache[key.String()] = valid
	bls.mut.Unlock()

	return err
}

func (bls *bls12Base) coreAggregateVerify(publicKeys []*BLS12PublicKey, messages [][]byte, signature *bls12.PointG2) error {
	n := len(publicKeys)
	// validate input
	if n != len(messages) {
		return fmt.Errorf("bls12: %d keys mismatch %d messages", n, len(messages))
	}

	// precondition n >= 1
	if n < 1 {
		return fmt.Errorf("bls12: expected at least one message")
	}

	if err := bls.subgroupCheck(signature); err != nil {
		return err
	}

	engine := bls12.NewEngine()

	for i := 0; i < n; i++ {
		q, err := engine.G2.HashToCurve(messages[i], domain)
		if err != nil {
			return err
		}
		engine.AddPair(publicKeys[i].p, q)
	}

	engine.AddPairInv(&bls12.G1One, signature)
	if !engine.Result().IsOne() {
		return fmt.Errorf("bls12: failed to verify aggregated message")
	}
	return nil
}

func (bls *bls12Base) aggregateVerify(publicKeys []*BLS12PublicKey, messages [][]byte, signature *bls12.PointG2) error {
	set := make(map[string]struct{})
	for _, m := range messages {
		set[string(m)] = struct{}{}
	}
	if len(messages) != len(set) {
		return fmt.Errorf("bls12: failed to verify aggregate: duplicate messages")
	}
	return bls.coreAggregateVerify(publicKeys, messages, signature)
}

func (bls *bls12Base) fastAggregateVerify(publicKeys []*BLS12PublicKey, message []byte, signature *bls12.PointG2) error {
	engine := bls12.NewEngine()
	var aggregate bls12.PointG1
	for _, pk := range publicKeys {
		engine.G1.Add(&aggregate, &aggregate, pk.p)
	}
	return bls.coreVerify(&BLS12PublicKey{p: &aggregate}, message, signature, domain)
}

// Sign creates a cryptographic signature of the given message.
func (bls *bls12Base) Sign(message []byte) (signature hotstuff.QuorumSignature, err error) {
	p, err := bls.coreSign(message, domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: coreSign failed: %w", err)
	}
	bf := Bitfield{}
	bf.Add(bls.config.ID())
	return &BLS12AggregateSignature{sig: *p, participants: bf}, nil
}

// Combine combines multiple signatures into a single signature.
func (bls *bls12Base) Combine(signatures ...hotstuff.QuorumSignature) (combined hotstuff.QuorumSignature, err error) {
	if len(signatures) < 2 {
		return nil, ErrCombineMultiple
	}

	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	var participants Bitfield
	for _, sig1 := range signatures {
		sig2, ok := sig1.(*BLS12AggregateSignature)
		if !ok {
			return nil, fmt.Errorf("bls12: cannot combine incompatible signature type %T (expected %T)", sig1, sig2)
		}
		sig2.participants.RangeWhile(func(id hotstuff.ID) bool {
			if participants.Contains(id) {
				err = ErrCombineOverlap
				return false
			}
			participants.Add(id)
			return true
		})
		if err != nil {
			return nil, err
		}
		g2.Add(&agg, &agg, &sig2.sig)
	}
	return &BLS12AggregateSignature{sig: agg, participants: participants}, nil
}

// Verify verifies the given quorum signature against the message.
func (bls *bls12Base) Verify(signature hotstuff.QuorumSignature, message []byte) error {
	s, ok := signature.(*BLS12AggregateSignature)
	if !ok {
		return fmt.Errorf("bls12: cannot verify signature of incompatible type %T (expected %T)", signature, s)
	}

	n := s.Participants().Len()

	if n == 1 {
		id := firstParticipant(s.Participants())
		pk, err := bls.publicKey(id)
		if err != nil {
			return err
		}
		if err := bls.coreVerify(pk, message, &s.sig, domain); err != nil {
			return err
		}
	}

	// else if l > 1:
	pks := make([]*BLS12PublicKey, 0, n)
	var errs error
	s.Participants().RangeWhile(func(id hotstuff.ID) bool {
		pk, err := bls.publicKey(id)
		if err != nil {
			errs = errors.Join(err)
			return false
		}
		pks = append(pks, pk)
		return true
	})
	if errs != nil {
		return fmt.Errorf("bls12: missing one or more public keys: %w", errs)
	}
	return bls.fastAggregateVerify(pks, message, &s.sig)
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (bls *bls12Base) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error {
	s, ok := signature.(*BLS12AggregateSignature)
	if !ok {
		return fmt.Errorf("bls12: cannot verify incompatible signature type %T (expected %T)", signature, s)
	}

	if s.Participants().Len() != len(batch) {
		return fmt.Errorf("bls12: signature mismatch: %d participants, expected: %d", len(batch), s.Participants().Len())
	}

	pks := make([]*BLS12PublicKey, 0, len(batch))
	msgs := make([][]byte, 0, len(batch))

	for id, msg := range batch {
		msgs = append(msgs, msg)
		pk, err := bls.publicKey(id)
		if err != nil {
			return err
		}
		pks = append(pks, pk)
	}

	if len(batch) == 1 {
		if err := bls.coreVerify(pks[0], msgs[0], &s.sig, domain); err != nil {
			return err
		}
		return nil
	}
	return bls.aggregateVerify(pks, msgs, &s.sig)
}

var _ Base = (*bls12Base)(nil)
