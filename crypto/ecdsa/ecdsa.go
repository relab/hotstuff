package ecdsa

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
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

type Signature struct {
	r, s   *big.Int
	signer hotstuff.ID
}

func NewSignature(r, s *big.Int, signer hotstuff.ID) Signature {
	return Signature{r, s, signer}
}

// Signer returns the ID of the replica that generated the signature.
func (sig Signature) Signer() hotstuff.ID {
	return sig.signer
}

func (sig Signature) R() *big.Int {
	return sig.r
}

func (sig Signature) S() *big.Int {
	return sig.s
}

// RawBytes returns a raw byte string representation of the signature
func (sig Signature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.r.Bytes()...)
	b = append(b, sig.s.Bytes()...)
	return b
}

var _ hotstuff.Signature = (*Signature)(nil)

type PartialCert struct {
	signature Signature
	hash      hotstuff.Hash
}

func NewPartialCert(signature Signature, hash hotstuff.Hash) PartialCert {
	return PartialCert{signature, hash}
}

// Signature returns the signature
func (cert PartialCert) Signature() hotstuff.Signature {
	return cert.signature
}

// BlockHash returns the hash of the block that was signed
func (cert PartialCert) BlockHash() hotstuff.Hash {
	return cert.hash
}

func (cert PartialCert) ToBytes() []byte {
	return append(cert.hash[:], cert.signature.ToBytes()...)
}

var _ hotstuff.PartialCert = (*PartialCert)(nil)

type QuorumCert struct {
	signatures map[hotstuff.ID]Signature
	hash       hotstuff.Hash
}

func NewQuorumCert(signatures map[hotstuff.ID]Signature, hash hotstuff.Hash) QuorumCert {
	return QuorumCert{signatures, hash}
}

func (qc QuorumCert) Signatures() map[hotstuff.ID]Signature {
	return qc.signatures
}

// BlockHash returns the hash of the block for which the certificate was created
func (qc QuorumCert) BlockHash() hotstuff.Hash {
	return qc.hash
}

func (qc QuorumCert) ToBytes() []byte {
	b := qc.hash[:]
	// sort signatures by id to ensure determinism
	sigs := make([]Signature, 0, len(qc.signatures))
	for _, sig := range qc.signatures {
		i := sort.Search(len(sigs), func(i int) bool {
			return sig.signer < sigs[i].signer
		})
		sigs = append(sigs, Signature{})
		copy(sigs[i+1:], sigs[i:])
		sigs[i] = sig
	}
	for _, sig := range sigs {
		b = append(b, sig.ToBytes()...)
	}
	return b
}

var _ hotstuff.QuorumCert = (*QuorumCert)(nil)

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
func (ec *ecdsaCrypto) Sign(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	hash := block.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey().PrivateKey, hash[:])
	if err != nil {
		return nil, err
	}
	return &PartialCert{
		Signature{r, s, ec.cfg.ID()},
		hash,
	}, nil
}

// CreateQuourmCert creates a from a list of partial certificates
func (ec *ecdsaCrypto) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	hash := block.Hash()
	qc := &QuorumCert{
		signatures: make(map[hotstuff.ID]Signature),
		hash:       hash,
	}
	for _, s := range signatures {
		blockHash := s.BlockHash()
		if !bytes.Equal(hash[:], blockHash[:]) {
			return nil, ErrHashMismatch
		}
		if _, ok := qc.signatures[s.Signature().Signer()]; ok {
			return nil, ErrPartialDuplicate
		}
		qc.signatures[s.Signature().Signer()] = s.(*PartialCert).signature
	}
	return qc, nil
}

// VerifyPartialCert verifies a single partial certificate
func (ec *ecdsaCrypto) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	// TODO: decide how to handle incompatible types. For now we'll simply panic
	sig := cert.Signature().(Signature)
	replica, ok := ec.cfg.Replica(sig.Signer())
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
	qc := cert.(*QuorumCert)
	if len(qc.Signatures()) < ec.cfg.QuorumSize() {
		return false
	}
	hash := qc.BlockHash()
	var wg sync.WaitGroup
	var numVerified uint64 = 0
	for id, psig := range qc.Signatures() {
		info, ok := ec.cfg.Replica(id)
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
