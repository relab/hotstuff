package proto

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

func SignatureToProto(sig hotstuff.Signature) *Signature {
	s := sig.(ecdsa.Signature)
	return &Signature{
		ReplicaID: uint32(s.Signer()),
		XR:        s.R().Bytes(),
		XS:        s.S().Bytes(),
	}
}

func PartialCertToProto(cert hotstuff.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  SignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
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

func BlockToProto(block hotstuff.Block) *Block {
	parentHash := block.Parent()
	return &Block{
		XParent:  parentHash[:],
		XCommand: []byte(block.Command()),
		QC:       QuorumCertToProto(block.QuorumCert()),
		XView:    uint64(block.View()),
	}
}
