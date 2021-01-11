package ecdsa

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/logging"
)

var logger = logging.GetLogger()

var ErrHashMismatch = fmt.Errorf("Certificate hash does not match block hash")
var ErrPartialDuplicate = fmt.Errorf("Cannot add more than one signature per replica")

type PrivateKey struct {
	*ecdsa.PrivateKey
}

func (pk PrivateKey) PublicKey() hotstuff.PublicKey {
	return pk.Public()
}

var _ hotstuff.PrivateKey = (*PrivateKey)(nil)

type Signature interface {
	hotstuff.Signature

	R() *big.Int
	S() *big.Int
}

type ecdsaSignature struct {
	r, s *big.Int
	id   hotstuff.ID
}

// Signer returns the ID of the replica that generated the signature.
func (sig ecdsaSignature) Signer() hotstuff.ID {
	return sig.id
}

func (sig *ecdsaSignature) R() *big.Int {
	return sig.r
}

func (sig *ecdsaSignature) S() *big.Int {
	return sig.s
}

// RawBytes returns a raw byte string representation of the signature
func (sig *ecdsaSignature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.r.Bytes()...)
	b = append(b, sig.s.Bytes()...)
	return b
}

var _ Signature = (*ecdsaSignature)(nil)

type ecdsaPartialCert struct {
	sig  Signature
	hash hotstuff.Hash
}

// Signature returns the signature
func (cert ecdsaPartialCert) Signature() hotstuff.Signature {
	return cert.sig
}

// BlockHash returns the hash of the block that was signed
func (cert ecdsaPartialCert) BlockHash() hotstuff.Hash {
	return cert.hash
}

func (cert ecdsaPartialCert) ToBytes() []byte {
	return append(cert.hash[:], cert.sig.ToBytes()...)
}

var _ hotstuff.PartialCert = (*ecdsaPartialCert)(nil)

type QuorumCert interface {
	hotstuff.QuorumCert

	Signatures() map[hotstuff.ID]Signature
}

type ecdsaQuorumCert struct {
	sigs map[hotstuff.ID]Signature
	hash hotstuff.Hash
}

func (qc ecdsaQuorumCert) Signatures() map[hotstuff.ID]Signature {
	return qc.sigs
}

// BlockHash returns the hash of the block for which the certificate was created
func (qc ecdsaQuorumCert) BlockHash() hotstuff.Hash {
	return qc.hash
}

func (qc ecdsaQuorumCert) ToBytes() []byte {
	b := qc.hash[:]
	for _, sig := range qc.sigs {
		b = append(b, sig.ToBytes()...)
	}
	return b
}

var _ QuorumCert = (*ecdsaQuorumCert)(nil)

// TODO: consider adding caching back

type ecdsaCrypto struct {
	cfg hotstuff.Config
}

func New(cfg hotstuff.Config) (hotstuff.Signer, hotstuff.Verifier) {
	ec := &ecdsaCrypto{cfg}
	return ec, ec
}

func (ec *ecdsaCrypto) getPrivateKey() *PrivateKey {
	pk := ec.cfg.PrivateKey()
	return pk.(*PrivateKey)
}

// Sign signs a single block and returns the signature
func (ec *ecdsaCrypto) Sign(block hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	hash := block.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey().PrivateKey, hash[:])
	if err != nil {
		return nil, err
	}
	return &ecdsaPartialCert{
		&ecdsaSignature{r, s, ec.cfg.ID()},
		hash,
	}, nil
}

// CreateQuourmCert creates a from a list of partial certificates
func (ec *ecdsaCrypto) CreateQuorumCert(block hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	hash := block.Hash()
	qc := &ecdsaQuorumCert{
		sigs: make(map[hotstuff.ID]Signature),
		hash: hash,
	}
	for _, s := range signatures {
		blockHash := s.BlockHash()
		if !bytes.Equal(hash[:], blockHash[:]) {
			return nil, ErrHashMismatch
		}
		if _, ok := qc.sigs[s.Signature().Signer()]; ok {
			return nil, ErrPartialDuplicate
		}
		qc.sigs[s.Signature().Signer()] = s.(Signature)
	}
	return qc, nil
}

// VerifyPartialCert verifies a single partial certificate
func (ec *ecdsaCrypto) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	// TODO: decide how to handle incompatible types. For now we'll simply panic
	sig := cert.Signature().(Signature)
	replica, ok := ec.cfg.Replicas()[sig.Signer()]
	if !ok {
		logger.Info("ecdsaCrypto: got signature from replica whose ID (%d) was not in the config.")
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	hash := cert.BlockHash()
	return ecdsa.Verify(pk, hash[:], sig.R(), sig.S())
}

// VerifyQuorumCert verifies a quorum certificate
func (ec *ecdsaCrypto) VerifyQuorumCert(cert hotstuff.QuorumCert) bool {
	qc := cert.(*ecdsaQuorumCert)
	if len(qc.sigs) < ec.cfg.QuorumSize() {
		return false
	}
	hash := qc.BlockHash()
	var wg sync.WaitGroup
	var numVerified uint64 = 0
	for id, psig := range qc.sigs {
		info, ok := ec.cfg.Replicas()[id]
		if !ok {
			logger.Error("VerifyQuorumSig: got signature from replica whose ID (%d) was not in config.", id)
		}
		pubKey := info.PublicKey().(*ecdsa.PublicKey)
		wg.Add(1)
		go func(psig Signature) {
			if ecdsa.Verify(pubKey, hash[:], psig.R(), psig.S()) {
				atomic.AddUint64(&numVerified, 1)
			}
			wg.Done()
		}(psig)
	}
	wg.Wait()
	return numVerified >= uint64(ec.cfg.QuorumSize())
}

var _ hotstuff.Signer = (*ecdsaCrypto)(nil)
var _ hotstuff.Verifier = (*ecdsaCrypto)(nil)
