package proto

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/data"
)

func SignatureToProto(sig hotstuff.Signature) *Signature {
	s := sig.(ecdsa.Signature)
	return &Signature{
		ReplicaID: uint32(s.Signer()),
		XR:        s.R().Bytes(),
		XS:        s.S().Bytes(),
	}
}

func (pps *Signature) FromProto() *data.PartialSig {
	r := big.NewInt(0)
	s := big.NewInt(0)
	r.SetBytes(pps.GetXR())
	s.SetBytes(pps.GetXS())
	return &data.PartialSig{
		ID: config.ReplicaID(pps.GetReplicaID()),
		R:  r,
		S:  s,
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
