// Package ecdsa provides a crypto implementation for HotStuff using Go's 'crypto/ecdsa' package.
package ecdsa

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/relab/hotstuff"
	"go.uber.org/multierr"
)

// ErrHashMismatch is the error used when a partial certificate hash does not match the hash of a block.
var ErrHashMismatch = fmt.Errorf("certificate hash does not match block hash")

// ErrPartialDuplicate is the error used when two or more signatures were created by the same replica.
var ErrPartialDuplicate = fmt.Errorf("cannot add more than one signature per replica")

// ErrViewMismatch is the error used when timeouts have different views.
var ErrViewMismatch = fmt.Errorf("timeout views do not match")

// ErrNotAQuorum is the error used when a q
var ErrNotAQuorum = fmt.Errorf("not a quorum")

// Signature is an ECDSA signature
type Signature struct {
	r, s   *big.Int
	signer hotstuff.ID
}

// NewSignature creates a new Signature struct from the given values.
func NewSignature(r, s *big.Int, signer hotstuff.ID) *Signature {
	return &Signature{r, s, signer}
}

// Signer returns the ID of the replica that generated the signature.
func (sig Signature) Signer() hotstuff.ID {
	return sig.signer
}

// R returns the r value of the signature
func (sig Signature) R() *big.Int {
	return sig.r
}

// S returns the s value of the signature
func (sig Signature) S() *big.Int {
	return sig.s
}

// ToBytes returns a raw byte string representation of the signature
func (sig Signature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.r.Bytes()...)
	b = append(b, sig.s.Bytes()...)
	return b
}

var _ hotstuff.Signature = (*Signature)(nil)

// PartialCert is an ECDSA signature and the hash that was signed.
type PartialCert struct {
	signature *Signature
	hash      hotstuff.Hash
}

// NewPartialCert initializes a PartialCert struct from the given values.
func NewPartialCert(signature *Signature, hash hotstuff.Hash) *PartialCert {
	return &PartialCert{signature, hash}
}

// Signature returns the signature.
func (cert PartialCert) Signature() hotstuff.Signature {
	return cert.signature
}

// BlockHash returns the hash of the block that was signed.
func (cert PartialCert) BlockHash() hotstuff.Hash {
	return cert.hash
}

// ToBytes returns a byte representation of the partial certificate.
func (cert PartialCert) ToBytes() []byte {
	return append(cert.hash[:], cert.signature.ToBytes()...)
}

func (cert PartialCert) String() string {
	return fmt.Sprintf("PartialCert{ Block: %.6s, signer: %d }", cert.hash.String(), cert.signature.signer)
}

var _ hotstuff.PartialCert = (*PartialCert)(nil)

type aggregateSignature map[hotstuff.ID]*Signature

func (agg aggregateSignature) ToBytes() (b []byte) {
	sigs := make([]*Signature, 0, len(agg))
	for _, sig := range agg {
		i := sort.Search(len(sigs), func(i int) bool {
			return sig.signer < sigs[i].signer
		})
		sigs = append(sigs, nil)
		copy(sigs[i+1:], sigs[i:])
		sigs[i] = sig
	}
	for _, sig := range sigs {
		b = append(b, sig.ToBytes()...)
	}
	return b
}

// QuorumCert is a set of signatures that form a quorum certificate for a block.
type QuorumCert struct {
	signatures aggregateSignature
	hash       hotstuff.Hash
}

// NewQuorumCert initializes a new QuorumCert struct from the given values.
func NewQuorumCert(signatures map[hotstuff.ID]*Signature, hash hotstuff.Hash) *QuorumCert {
	return &QuorumCert{signatures, hash}
}

// Signatures returns the signatures within the quorum certificate.
func (qc QuorumCert) Signatures() map[hotstuff.ID]*Signature {
	return qc.signatures
}

// BlockHash returns the hash of the block for which the certificate was created.
func (qc QuorumCert) BlockHash() hotstuff.Hash {
	return qc.hash
}

// ToBytes returns a byte representation of the quorum certificate.
func (qc QuorumCert) ToBytes() (b []byte) {
	b = append(b, qc.hash[:]...)
	b = append(b, qc.signatures.ToBytes()...)
	return b
}

func (qc QuorumCert) String() string {
	var sb strings.Builder
	for _, sig := range qc.signatures {
		sb.WriteString(" " + strconv.Itoa(int(sig.signer)) + " ")
	}
	return fmt.Sprintf("QC{ Block: %.6s, Sigs: [%s] }", qc.hash.String(), sb.String())
}

var _ hotstuff.QuorumCert = (*QuorumCert)(nil)

// TimeoutCert is a set of signatures that form a quorum certificate for a timed out view.
type TimeoutCert struct {
	signatures aggregateSignature
	view       hotstuff.View
}

// NewTimeoutCert initializes a new TimeoutCert struct from the given values.
func NewTimeoutCert(signatures map[hotstuff.ID]*Signature, view hotstuff.View) *TimeoutCert {
	return &TimeoutCert{signatures, view}
}

// Signatures returns the set of signatures in the timeout certificate.
func (tc TimeoutCert) Signatures() map[hotstuff.ID]*Signature {
	return tc.signatures
}

// ToBytes returns the object as bytes.
func (tc TimeoutCert) ToBytes() (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(tc.view))
	b = append(b, tc.signatures.ToBytes()...)
	return b
}

// View returns the view that timed out.
func (tc TimeoutCert) View() hotstuff.View {
	return tc.view
}

type ecdsaCrypto struct {
	mod *hotstuff.HotStuff
}

func (ec *ecdsaCrypto) InitModule(hs *hotstuff.HotStuff) {
	ec.mod = hs
}

// New returns a new signer and a new verifier.
func New() (hotstuff.Signer, hotstuff.Verifier) {
	ec := &ecdsaCrypto{}
	return ec, ec
}

func (ec *ecdsaCrypto) getPrivateKey() *ecdsa.PrivateKey {
	pk := ec.mod.PrivateKey()
	return pk.(*ecdsa.PrivateKey)
}

// Sign signs a hash.
func (ec *ecdsaCrypto) Sign(hash hotstuff.Hash) (sig hotstuff.Signature, err error) {
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, err
	}
	return &Signature{
		r:      r,
		s:      s,
		signer: ec.mod.ID(),
	}, nil
}

// Sign signs a single block and returns a partial certificate.
func (ec *ecdsaCrypto) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	hash := block.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, ec.getPrivateKey(), hash[:])
	if err != nil {
		return nil, err
	}
	return &PartialCert{
		&Signature{r, s, ec.mod.ID()},
		hash,
	}, nil
}

// CreateQuorumCert creates a quorum certificate from a block and a set of signatures.
func (ec *ecdsaCrypto) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	hash := block.Hash()
	qc := &QuorumCert{
		signatures: make(aggregateSignature),
		hash:       hash,
	}
	// it will always be possible to create a valid QC for genesis block
	if hash == hotstuff.GetGenesis().Hash() {
		return qc, nil
	}
	for _, s := range signatures {
		blockHash := s.BlockHash()
		if !bytes.Equal(hash[:], blockHash[:]) {
			err = multierr.Append(err, ErrHashMismatch)
			continue
		}
		if _, ok := qc.signatures[s.Signature().Signer()]; ok {
			err = multierr.Append(err, ErrPartialDuplicate)
			continue
		}
		// use the registered verifier instead of ourself to verify.
		// this makes it possible for the signatureCache to work.
		if ec.mod.Verifier().VerifyPartialCert(s) {
			qc.signatures[s.Signature().Signer()] = s.(*PartialCert).signature
		}
	}

	if len(qc.signatures) >= ec.mod.Config().QuorumSize() {
		return qc, nil
	}

	return nil, multierr.Combine(ErrNotAQuorum, err)
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (ec *ecdsaCrypto) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
	tc := &TimeoutCert{
		signatures: make(aggregateSignature),
		view:       view,
	}
	// it will always be possible to create a TC for view 0
	if view == 0 {
		return tc, nil
	}
	for _, t := range timeouts {
		if t.View != tc.view {
			err = multierr.Append(err, ErrHashMismatch)
			continue
		}
		if _, ok := tc.signatures[t.Signature.Signer()]; ok {
			err = multierr.Append(err, ErrPartialDuplicate)
			continue
		}
		// use the registered verifier instead of ourself to verify.
		// this makes it possible for the signatureCache to work.
		if ec.mod.Verifier().Verify(t.Signature, view.ToHash()) {
			tc.signatures[t.Signature.Signer()] = t.Signature.(*Signature)
		}
	}
	if len(tc.signatures) >= ec.mod.Config().QuorumSize() {
		return tc, nil
	}

	return nil, multierr.Combine(ErrNotAQuorum, err)
}

// Verify verifies a signature given a hash.
func (ec *ecdsaCrypto) Verify(sig hotstuff.Signature, hash hotstuff.Hash) bool {
	_sig := sig.(*Signature)
	replica, ok := ec.mod.Config().Replica(sig.Signer())
	if !ok {
		ec.mod.Logger().Info("ecdsaCrypto: got signature from replica whose ID (%d) was not in the config.")
		return false
	}
	pk := replica.PublicKey().(*ecdsa.PublicKey)
	return ecdsa.Verify(pk, hash[:], _sig.R(), _sig.S())
}

// VerifyPartialCert verifies a single partial certificate.
func (ec *ecdsaCrypto) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	// TODO: decide how to handle incompatible types. For now we'll simply panic
	sig := cert.Signature().(*Signature)
	return ec.Verify(sig, cert.BlockHash())
}

func (ec *ecdsaCrypto) verifyAggregateSignature(agg aggregateSignature, hash hotstuff.Hash) bool {
	if len(agg) < ec.mod.Config().QuorumSize() {
		return false
	}
	var numVerified uint32
	var wg sync.WaitGroup
	wg.Add(len(agg))
	for _, pSig := range agg {
		go func(sig *Signature) {
			if ec.Verify(sig, hash) {
				atomic.AddUint32(&numVerified, 1)
			}
			wg.Done()
		}(pSig)
	}
	wg.Wait()
	return numVerified >= uint32(ec.mod.Config().QuorumSize())
}

// VerifyQuorumCert verifies a quorum certificate.
func (ec *ecdsaCrypto) VerifyQuorumCert(cert hotstuff.QuorumCert) bool {
	// If QC was created for genesis, then skip verification.
	if cert.BlockHash() == hotstuff.GetGenesis().Hash() {
		return true
	}

	qc := cert.(*QuorumCert)
	return ec.verifyAggregateSignature(qc.signatures, qc.hash)
}

// VerifyTimeoutCert verifies a timeout certificate.
func (ec *ecdsaCrypto) VerifyTimeoutCert(cert hotstuff.TimeoutCert) bool {
	tc := cert.(*TimeoutCert)
	return ec.verifyAggregateSignature(tc.signatures, tc.view.ToHash())
}

var _ hotstuff.Signer = (*ecdsaCrypto)(nil)
var _ hotstuff.Verifier = (*ecdsaCrypto)(nil)
