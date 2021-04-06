// Package bls12 implements the crypto primitives used by HotStuff using curve BLS12-381.
package bls12

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"

	bls12 "github.com/kilic/bls12-381"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"go.uber.org/multierr"
)

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
	return err
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
func (priv PrivateKey) FromBytes(b []byte) {
	priv.p = new(big.Int)
	priv.p.SetBytes(b)
}

// GeneratePrivateKey generates a new private key.
func GeneratePrivateKey() (*PrivateKey, error) {
	// the private key is uniformly random integer such that 0 <= pk < r
	pk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		return nil, err
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

func (s *Signature) FromBytes(b []byte) (err error) {
	s.signer = hotstuff.ID(binary.LittleEndian.Uint32(b))
	s.s, err = bls12.NewG2().FromCompressed(b[3:])
	return err
}

// Signer returns the ID of the replica that generated the signature.
func (s *Signature) Signer() hotstuff.ID {
	return s.signer
}

type PartialCert struct {
	sig  Signature
	hash hotstuff.Hash
}

// ToBytes returns the object as bytes.
func (p *PartialCert) ToBytes() []byte {
	return append(p.hash[:], p.sig.ToBytes()...)
}

// Signature returns the signature of the block.
func (p *PartialCert) Signature() hotstuff.Signature {
	return &p.sig
}

// BlockHash returns the hash of the block that was signed.
func (p *PartialCert) BlockHash() hotstuff.Hash {
	return p.hash
}

type aggregateSignature struct {
	sig          bls12.PointG2
	participants map[hotstuff.ID]struct{} // The ids of the replicas who submitted signatures.
}

// ToBytes returns a byte representation of the aggregate signature.
func (agg *aggregateSignature) ToBytes() []byte {
	b := bls12.NewG2().ToCompressed(&agg.sig)
	for id := range agg.participants {
		var t [4]byte
		binary.LittleEndian.PutUint32(t[:], uint32(id))
		b = append(b, t[:]...)
	}
	return b
}

type QuorumCert struct {
	sig  *aggregateSignature
	hash hotstuff.Hash
}

// ToBytes returns the object as bytes.
func (qc *QuorumCert) ToBytes() []byte {
	return append(qc.sig.ToBytes(), qc.hash[:]...)
}

// BlockHash returns the hash of the block that the QC was created for.
func (qc *QuorumCert) BlockHash() hotstuff.Hash {
	return qc.hash
}

type TimeoutCert struct {
	sig  *aggregateSignature
	view hotstuff.View
}

// ToBytes returns the object as bytes.
func (tc *TimeoutCert) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(tc.view))
	return append(viewBytes[:], tc.sig.ToBytes()...)
}

// View returns the view that timed out.
func (tc *TimeoutCert) View() hotstuff.View {
	return tc.view
}

type bls12Crypto struct {
	mod *hotstuff.HotStuff
}

func New() *bls12Crypto {
	return &bls12Crypto{}
}

func (bc *bls12Crypto) getPrivateKey() *PrivateKey {
	pk := bc.mod.PrivateKey()
	return pk.(*PrivateKey)
}

// InitModule gives the module a reference to the HotStuff object.
func (bc *bls12Crypto) InitModule(hs *hotstuff.HotStuff) {
	bc.mod = hs
}

// Sign signs a hash.
func (bc *bls12Crypto) Sign(hash hotstuff.Hash) (sig hotstuff.Signature, err error) {
	p, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		return nil, err
	}
	pk := bc.getPrivateKey()
	bls12.NewG2().MulScalarBig(p, p, pk.p)
	return &Signature{signer: bc.mod.ID(), s: p}, nil
}

func (bc *bls12Crypto) aggregateSignatures(signatures map[hotstuff.ID]*Signature) *aggregateSignature {
	if len(signatures) == 0 {
		return nil
	}
	g2 := bls12.NewG2()
	sig := bls12.PointG2{}
	participants := make(map[hotstuff.ID]struct{}, len(signatures))
	for id, s := range signatures {
		g2.Add(&sig, &sig, s.s)
		participants[id] = struct{}{}
	}
	return &aggregateSignature{sig: sig, participants: participants}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (bc *bls12Crypto) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := bc.Sign(block.Hash())
	if err != nil {
		return nil, err
	}
	return &PartialCert{sig: *sig.(*Signature), hash: block.Hash()}, nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (bc *bls12Crypto) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	if block.Hash() == hotstuff.GetGenesis().Hash() {
		return &QuorumCert{hash: hotstuff.GetGenesis().Hash()}, nil
	}
	if len(signatures) < bc.mod.Config().QuorumSize() {
		return nil, ecdsa.ErrNotAQuorum
	}
	sigs := make(map[hotstuff.ID]*Signature, len(signatures))
	for _, sig := range signatures {
		if sig.BlockHash() != block.Hash() {
			err = multierr.Append(err, ecdsa.ErrHashMismatch)
			continue
		}
		if _, ok := sigs[sig.Signature().Signer()]; ok {
			err = multierr.Append(err, ecdsa.ErrPartialDuplicate)
			continue
		}
		sigs[sig.Signature().Signer()] = &sig.(*PartialCert).sig
	}
	if len(sigs) < bc.mod.Config().QuorumSize() {
		return nil, multierr.Combine(ecdsa.ErrNotAQuorum, err)
	}
	aggSig := bc.aggregateSignatures(sigs)
	return &QuorumCert{sig: aggSig, hash: block.Hash()}, nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (bc *bls12Crypto) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
	if view == 0 {
		return &TimeoutCert{view: 0}, nil
	}
	if len(timeouts) < bc.mod.Config().QuorumSize() {
		return nil, ecdsa.ErrNotAQuorum
	}
	sigs := make(map[hotstuff.ID]*Signature, len(timeouts))
	for _, timeout := range timeouts {
		if timeout.View != view {
			err = multierr.Append(err, ecdsa.ErrHashMismatch)
			continue
		}
		if _, ok := sigs[timeout.ID]; ok {
			err = multierr.Append(err, ecdsa.ErrPartialDuplicate)
			continue
		}
		sigs[timeout.ID] = timeout.Signature.(*Signature)
	}
	if len(sigs) < bc.mod.Config().QuorumSize() {
		return nil, multierr.Combine(ecdsa.ErrNotAQuorum, err)
	}
	aggSig := bc.aggregateSignatures(sigs)
	return &TimeoutCert{sig: aggSig, view: view}, nil
}

// Verify verifies a signature given a hash.
func (bc *bls12Crypto) Verify(sig hotstuff.Signature, hash hotstuff.Hash) bool {
	s := sig.(*Signature)
	replica, ok := bc.mod.Config().Replica(sig.Signer())
	if !ok {
		bc.mod.Logger().Infof("bls12Crypto: got signature from replica whose ID (%d) was not in the config", sig.Signer())
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

// TODO: I'm not sure to what extent we are vulnerable to a rogue public key attack here.
// As far as I can tell, this is not a problem right now because we do not yet support reconfiguration,
// and all public keys are known by all replicas.
func (bc *bls12Crypto) fastAggregateVerify(sig *aggregateSignature, hash hotstuff.Hash) bool {
	pubKeys := make([]*PublicKey, 0, len(sig.participants))
	for id := range sig.participants {
		replica, ok := bc.mod.Config().Replica(id)
		if !ok {
			return false
		}
		pubKeys = append(pubKeys, replica.PublicKey().(*PublicKey))
	}
	ps, err := bls12.NewG2().HashToCurve(hash[:], domain)
	if err != nil {
		bc.mod.Logger().Error(err)
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, &sig.sig)
	for _, pub := range pubKeys {
		engine.AddPair(pub.p, ps)
	}
	return engine.Result().IsOne()
}

// VerifyPartialCert verifies a single partial certificate.
func (bc *bls12Crypto) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	c := cert.(*PartialCert)
	return bc.Verify(&c.sig, c.hash)
}

// VerifyQuorumCert verifies a quorum certificate.
func (bc *bls12Crypto) VerifyQuorumCert(qc hotstuff.QuorumCert) bool {
	q := qc.(*QuorumCert)
	return bc.fastAggregateVerify(q.sig, q.hash)
}

// VerifyTimeoutCert verifies a timeout certificate.
func (bc *bls12Crypto) VerifyTimeoutCert(tc hotstuff.TimeoutCert) bool {
	t := tc.(*TimeoutCert)
	return bc.fastAggregateVerify(t.sig, t.view.ToHash())
}
