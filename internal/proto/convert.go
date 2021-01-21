package proto

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

func SignatureToProto(sig hotstuff.Signature) *Signature {
	s := sig.(*ecdsa.Signature)
	return &Signature{
		ReplicaID: uint32(s.Signer()),
		R:         s.R().Bytes(),
		S:         s.S().Bytes(),
	}
}

func SignatureFromProto(sig *Signature) ecdsa.Signature {
	r := new(big.Int)
	r.SetBytes(sig.GetR())
	s := new(big.Int)
	s.SetBytes(sig.GetS())

	return ecdsa.NewSignature(r, s, hotstuff.ID(sig.GetReplicaID()))
}

func PartialCertToProto(cert hotstuff.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  SignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
}

func ParitalCertFromProto(cert *PartialCert) ecdsa.PartialCert {
	var h hotstuff.Hash
	copy(h[:], cert.GetHash())
	return ecdsa.NewPartialCert(SignatureFromProto(cert.GetSig()), h)
}

func QuorumCertToProto(qc hotstuff.QuorumCert) *QuorumCert {
	c := qc.(ecdsa.QuorumCert)
	sigs := make(map[uint32]*Signature, len(c.Signatures()))
	for id, psig := range c.Signatures() {
		sigs[uint32(id)] = SignatureToProto(psig)
	}
	hash := c.BlockHash()
	return &QuorumCert{
		Sigs: sigs,
		Hash: hash[:],
	}
}

func QuorumCertFromProto(qc *QuorumCert) ecdsa.QuorumCert {
	var h hotstuff.Hash
	copy(h[:], qc.GetHash())
	sigs := make(map[hotstuff.ID]ecdsa.Signature, len(qc.GetSigs()))
	for k, sig := range qc.GetSigs() {
		sigs[hotstuff.ID(k)] = SignatureFromProto(sig)
	}
	return ecdsa.NewQuorumCert(sigs, h)
}

func BlockToProto(block *hotstuff.Block) *Block {
	parentHash := block.Parent()
	return &Block{
		Parent:  parentHash[:],
		Command: []byte(block.Command()),
		QC:      QuorumCertToProto(block.QuorumCert()),
		View:    uint64(block.View()),
	}
}

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
