package proto

import (
	"crypto/sha256"
	"encoding/binary"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

func (sig *Signature) ToBytes() []byte {
	return append(sig.GetXR(), sig.GetXS()...)
}

// Signer returns the ID of the replica that generated the signature.
func (sig *Signature) Signer() hotstuff.ID {
	return hotstuff.ID(sig.ReplicaID)
}

func (sig *Signature) R() *big.Int {
	r := new(big.Int)
	r.SetBytes(sig.GetXR())
	return r
}

func (sig *Signature) S() *big.Int {
	s := new(big.Int)
	s.SetBytes(sig.GetXS())
	return s
}

var _ ecdsa.Signature = (*Signature)(nil)

func (cert *PartialCert) ToBytes() []byte {
	return append(cert.GetHash()[:], cert.GetSig().ToBytes()...)
}

// Signature returns the signature
func (cert *PartialCert) Signature() hotstuff.Signature {
	return cert.GetSig()
}

// BlockHash returns the hash of the block that was signed
func (cert *PartialCert) BlockHash() hotstuff.Hash {
	var hash hotstuff.Hash
	copy(hash[:], cert.GetHash())
	return hash
}

var _ hotstuff.PartialCert = (*PartialCert)(nil)

func (qc *QuorumCert) ToBytes() []byte {
	b := qc.GetHash()
	for _, sig := range qc.Sigs {
		b = append(b, sig.ToBytes()...)
	}
	return b
}

// BlockHash returns the hash of the block for which the certificate was created
func (qc *QuorumCert) BlockHash() hotstuff.Hash {
	var hash hotstuff.Hash
	copy(hash[:], qc.GetHash())
	return hash
}

func (qc *QuorumCert) Signatures() map[hotstuff.ID]ecdsa.Signature {
	sigs := make(map[hotstuff.ID]ecdsa.Signature, len(qc.GetSigs()))
	for k, v := range qc.GetSigs() {
		sigs[hotstuff.ID(k)] = v
	}
	return sigs
}

var _ ecdsa.QuorumCert = (*QuorumCert)(nil)

func (block *Block) ToBytes() []byte {
	buf := block.GetXParent()
	var proposerBuf [4]byte
	binary.LittleEndian.PutUint32(proposerBuf[:], block.GetXProposer())
	buf = append(buf, proposerBuf[:]...)
	var viewBuf [8]byte
	binary.LittleEndian.PutUint64(viewBuf[:], uint64(block.GetXView()))
	buf = append(buf, viewBuf[:]...)
	buf = append(buf, block.GetXCommand()...)
	buf = append(buf, block.GetQC().ToBytes()...)
	return buf
}

// Hash returns the hash of the block
func (block *Block) Hash() hotstuff.Hash {
	return sha256.Sum256(block.ToBytes())
}

// Proposer returns the id of the proposer
func (block *Block) Proposer() hotstuff.ID {
	return hotstuff.ID(block.GetXProposer())
}

// Parent returns the hash of the parent block
func (block *Block) Parent() hotstuff.Hash {
	var hash hotstuff.Hash
	copy(hash[:], block.GetXParent())
	return hash
}

// Command returns the command
func (block *Block) Command() hotstuff.Command {
	return hotstuff.Command(block.GetXCommand())
}

// Certificate returns the certificate that this block references
func (block *Block) QuorumCert() hotstuff.QuorumCert {
	return block.GetQC()
}

// View returns the view in which the block was proposed
func (block *Block) View() hotstuff.View {
	return hotstuff.View(block.GetXView())
}

var _ hotstuff.Block = (*Block)(nil)
