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
		Hash: p.BlockHash[:],
	}
}

func (ppc *PartialCert) FromProto() *hotstuff.PartialCert {
	pc := &hotstuff.PartialCert{
		Sig: *ppc.GetSig().FromProto(),
	}
	copy(pc.BlockHash[:], ppc.GetHash())
	return pc
}

func QuorumCertToProto(qc *hotstuff.QuorumCert) *QuorumCert {
	sigs := make([]*PartialSig, 0, len(qc.Sigs))
	for _, psig := range qc.Sigs {
		sigs = append(sigs, PartialSigToProto(&psig))
	}
	return &QuorumCert{
		Sigs: sigs,
		Hash: qc.BlockHash[:],
	}
}

func (pqc *QuorumCert) FromProto() *hotstuff.QuorumCert {
	qc := &hotstuff.QuorumCert{
		Sigs: make(map[hotstuff.ReplicaID]hotstuff.PartialSig),
	}
	copy(qc.BlockHash[:], pqc.GetHash())
	for _, ppsig := range pqc.GetSigs() {
		psig := ppsig.FromProto()
		qc.Sigs[psig.ID] = *psig
	}
	return qc
}

func BlockToProto(n *hotstuff.Block) *Block {
	commands := make([]*Command, 0, len(n.Commands))
	for _, cmd := range n.Commands {
		commands = append(commands, CommandToProto(cmd))
	}
	return &Block{
		ParentHash: n.ParentHash[:],
		Commands:   commands,
		QC:         QuorumCertToProto(n.Justify),
		Height:     int64(n.Height),
	}
}

func (pn *Block) FromProto() *hotstuff.Block {
	commands := make([]hotstuff.Command, 0, len(pn.GetCommands()))
	for _, cmd := range pn.GetCommands() {
		commands = append(commands, cmd.FromProto())
	}
	n := &hotstuff.Block{
		Justify:  pn.GetQC().FromProto(),
		Height:   int(pn.Height),
		Commands: commands,
	}
	copy(n.ParentHash[:], pn.GetParentHash())
	return n
}

func CommandToProto(cmd hotstuff.Command) *Command {
	return &Command{Data: []byte(cmd)}
}

func (cmd *Command) FromProto() hotstuff.Command {
	return hotstuff.Command(cmd.GetData())
}
