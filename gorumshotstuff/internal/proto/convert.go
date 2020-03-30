package proto

import (
	"math/big"

	"github.com/relab/hotstuff"
)

func PartialSigToProto(p *hotstuff.PartialSig) *PartialSig {
	r := p.R.Bytes()
	s := p.S.Bytes()
	return &PartialSig{
		ReplicaID: int32(p.ID),
		R:         r,
		S:         s,
	}
}

func (pps *PartialSig) FromProto() *hotstuff.PartialSig {
	r := big.NewInt(0)
	s := big.NewInt(0)
	r.SetBytes(pps.GetR())
	s.SetBytes(pps.GetS())
	return &hotstuff.PartialSig{
		ID: hotstuff.ReplicaID(pps.GetReplicaID()),
		R:  r,
		S:  s,
	}
}

func PartialCertToProto(p *hotstuff.PartialCert) *PartialCert {
	return &PartialCert{
		Sig:  PartialSigToProto(&p.Sig),
		Hash: p.NodeHash[:],
	}
}

func (ppc *PartialCert) FromProto() *hotstuff.PartialCert {
	pc := &hotstuff.PartialCert{
		Sig: *ppc.GetSig().FromProto(),
	}
	copy(pc.NodeHash[:], ppc.GetHash())
	return pc
}

func QuorumCertToProto(qc *hotstuff.QuorumCert) *QuorumCert {
	sigs := make([]*PartialSig, 0, len(qc.Sigs))
	for _, psig := range qc.Sigs {
		sigs = append(sigs, PartialSigToProto(&psig))
	}
	return &QuorumCert{
		Sigs: sigs,
		Hash: qc.NodeHash[:],
	}
}

func (pqc *QuorumCert) FromProto() *hotstuff.QuorumCert {
	qc := &hotstuff.QuorumCert{
		Sigs: make(map[hotstuff.ReplicaID]hotstuff.PartialSig),
	}
	copy(qc.NodeHash[:], pqc.GetHash())
	for _, ppsig := range pqc.GetSigs() {
		psig := ppsig.FromProto()
		qc.Sigs[psig.ID] = *psig
	}
	return qc
}

func NodeToProto(n *hotstuff.Node) *HSNode {
	return &HSNode{
		ParentHash: n.ParentHash[:],
		Command:    n.Command,
		QC:         QuorumCertToProto(n.Justify),
		Height:     int64(n.Height),
	}
}

func (pn *HSNode) FromProto() *hotstuff.Node {
	n := &hotstuff.Node{
		Command: pn.GetCommand(),
		Justify: pn.GetQC().FromProto(),
		Height:  int(pn.Height),
	}
	copy(n.ParentHash[:], pn.GetParentHash())
	return n
}
