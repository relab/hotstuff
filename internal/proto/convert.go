package proto

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

// SignatureToProto converts a hotstuff.Signature to a proto.Signature.
func SignatureToProto(sig hotstuff.Signature) *Signature {
	s := sig.(*ecdsa.Signature)
	return &Signature{
		ReplicaID: uint32(s.Signer()),
		R:         s.R().Bytes(),
		S:         s.S().Bytes(),
	}
}

// SignatureFromProto converts a proto.Signature to an ecdsa.Signature.
func SignatureFromProto(sig *Signature) *ecdsa.Signature {
	r := new(big.Int)
	r.SetBytes(sig.GetR())
	s := new(big.Int)
	s.SetBytes(sig.GetS())

	return ecdsa.NewSignature(r, s, hotstuff.ID(sig.GetReplicaID()))
}

// PartialCertToProto converts a hotstuff.PartialCert to a proto.Partialcert.
func PartialCertToProto(cert hotstuff.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  SignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
}

// PartialCertFromProto converts a proto.PartialCert to an ecdsa.PartialCert.
func PartialCertFromProto(cert *PartialCert) *ecdsa.PartialCert {
	var h hotstuff.Hash
	copy(h[:], cert.GetHash())
	return ecdsa.NewPartialCert(SignatureFromProto(cert.GetSig()), h)
}

// QuorumCertToProto converts a hotstuff.QuorumCert to a proto.QuorumCert.
func QuorumCertToProto(qc hotstuff.QuorumCert) *QuorumCert {
	c := qc.(*ecdsa.QuorumCert)
	sigs := make(map[uint32]*Signature, len(c.Signatures()))
	for id, pSig := range c.Signatures() {
		sigs[uint32(id)] = SignatureToProto(pSig)
	}
	hash := c.BlockHash()
	return &QuorumCert{
		Sigs: sigs,
		Hash: hash[:],
	}
}

// QuorumCertFromProto converts a proto.QuorumCert to an ecdsa.QuorumCert.
func QuorumCertFromProto(qc *QuorumCert) *ecdsa.QuorumCert {
	var h hotstuff.Hash
	copy(h[:], qc.GetHash())
	sigs := make(map[hotstuff.ID]*ecdsa.Signature, len(qc.GetSigs()))
	for k, sig := range qc.GetSigs() {
		sigs[hotstuff.ID(k)] = SignatureFromProto(sig)
	}
	return ecdsa.NewQuorumCert(sigs, h)
}

// BlockToProto converts a hotstuff.Block to a proto.Block.
func BlockToProto(block *hotstuff.Block) *Block {
	parentHash := block.Parent()
	return &Block{
		Parent:   parentHash[:],
		Command:  []byte(block.Command()),
		QC:       QuorumCertToProto(block.QuorumCert()),
		View:     uint64(block.View()),
		Proposer: uint32(block.Proposer()),
	}
}

// BlockFromProto converts a proto.Block to a hotstuff.Block.
func BlockFromProto(block *Block) *hotstuff.Block {
	var p hotstuff.Hash
	copy(p[:], block.GetParent())
	return hotstuff.NewBlock(
		p,
		QuorumCertFromProto(block.GetQC()),
		hotstuff.Command(block.GetCommand()),
		hotstuff.View(block.GetView()),
		hotstuff.ID(block.GetProposer()),
	)
}
