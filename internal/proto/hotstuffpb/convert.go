// Package hotstuffpb contains conversion functions between protocol buffer message types and HotStuff protocol message structures.
package hotstuffpb

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/security/crypto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// QuorumSignatureToProto converts a threshold signature to a protocol buffers message.
func QuorumSignatureToProto(sig hotstuff.QuorumSignature) *QuorumSignature {
	signature := &QuorumSignature{}
	switch ms := sig.(type) {
	case crypto.Multi[*crypto.ECDSASignature]:
		sigs := make([]*ECDSASignature, 0, sig.Participants().Len())
		for _, s := range ms {
			sigs = append(sigs, &ECDSASignature{
				Signer: uint32(s.Signer()),
				Sig:    s.ToBytes(),
			})
		}
		signature.Sig = &QuorumSignature_ECDSASigs{ECDSASigs: &ECDSAMultiSignature{
			Sigs: sigs,
		}}

	case crypto.Multi[*crypto.EDDSASignature]:
		sigs := make([]*EDDSASignature, 0, sig.Participants().Len())
		for _, s := range ms {
			sigs = append(sigs, &EDDSASignature{Signer: uint32(s.Signer()), Sig: s.ToBytes()})
		}
		signature.Sig = &QuorumSignature_EDDSASigs{EDDSASigs: &EDDSAMultiSignature{
			Sigs: sigs,
		}}

	case *crypto.BLS12AggregateSignature:
		signature.Sig = &QuorumSignature_BLS12Sig{BLS12Sig: &BLS12AggregateSignature{
			Sig:          ms.ToBytes(),
			Participants: ms.Bitfield().Bytes(),
		}}
	}
	return signature
}

// QuorumSignatureFromProto converts a protocol buffers message to a threshold signature.
func QuorumSignatureFromProto(sig *QuorumSignature) hotstuff.QuorumSignature {
	if signature := sig.GetECDSASigs(); signature != nil {
		sigs := make([]*crypto.ECDSASignature, len(signature.GetSigs()))
		for i, sig := range signature.GetSigs() {
			sigs[i] = crypto.RestoreECDSASignature(sig.GetSig(), hotstuff.ID(sig.GetSigner()))
		}
		return crypto.NewMulti(sigs...)
	}
	if signature := sig.GetEDDSASigs(); signature != nil {
		sigs := make([]*crypto.EDDSASignature, len(signature.GetSigs()))
		for i, sig := range signature.GetSigs() {
			sigs[i] = crypto.RestoreEDDSASignature(sig.Sig, hotstuff.ID(sig.GetSigner()))
		}
		return crypto.NewMulti(sigs...)
	}
	if signature := sig.GetBLS12Sig(); signature != nil {
		aggSig, err := crypto.RestoreBLS12AggregateSignature(signature.GetSig(), crypto.BitfieldFromBytes(signature.GetParticipants()))
		if err != nil {
			return nil
		}
		return aggSig
	}
	return nil
}

// PartialCertToProto converts a hotstuff.PartialCert to a PartialCert.
func PartialCertToProto(cert hotstuff.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  QuorumSignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
}

// PartialCertFromProto converts a PartialCert to a hotstuff.PartialCert.
func PartialCertFromProto(cert *PartialCert) hotstuff.PartialCert {
	var h hotstuff.Hash
	copy(h[:], cert.GetHash())
	return hotstuff.NewPartialCert(QuorumSignatureFromProto(cert.GetSig()), h)
}

// QuorumCertToProto converts a hotstuff.QuorumCert to a QuorumCert.
func QuorumCertToProto(qc hotstuff.QuorumCert) *QuorumCert {
	hash := qc.BlockHash()
	return &QuorumCert{
		Sig:  QuorumSignatureToProto(qc.Signature()),
		Hash: hash[:],
		View: uint64(qc.View()),
	}
}

// QuorumCertFromProto converts a QuorumCert to a hotstuff.QuorumCert.
func QuorumCertFromProto(qc *QuorumCert) hotstuff.QuorumCert {
	var h hotstuff.Hash
	copy(h[:], qc.GetHash())
	return hotstuff.NewQuorumCert(QuorumSignatureFromProto(qc.GetSig()), hotstuff.View(qc.GetView()), h)
}

// ProposalToProto converts a hotstuff.ProposeMsg to a Proposal.
func ProposalToProto(proposal hotstuff.ProposeMsg) *Proposal {
	p := &Proposal{
		Block: BlockToProto(proposal.Block),
	}
	if proposal.AggregateQC != nil {
		p.AggQC = AggregateQCToProto(*proposal.AggregateQC)
	}
	return p
}

// ProposalFromProto converts a Proposal to a hotstuff.ProposeMsg.
func ProposalFromProto(p *Proposal) (proposal hotstuff.ProposeMsg) {
	proposal.Block = BlockFromProto(p.GetBlock())
	if p.GetAggQC() != nil {
		aggQC := AggregateQCFromProto(p.GetAggQC())
		proposal.AggregateQC = &aggQC
	}
	return
}

// BlockToProto converts a hotstuff.Block to a Block.
func BlockToProto(block *hotstuff.Block) *Block {
	parentHash := block.Parent()
	return &Block{
		Parent:    parentHash[:],
		Commands:  block.Commands(),
		QC:        QuorumCertToProto(block.QuorumCert()),
		View:      uint64(block.View()),
		Proposer:  uint32(block.Proposer()),
		Timestamp: timestamppb.New(block.Timestamp()),
	}
}

// BlockFromProto converts a Block to a hotstuff.Block.
func BlockFromProto(block *Block) *hotstuff.Block {
	var p hotstuff.Hash
	copy(p[:], block.GetParent())

	b := hotstuff.NewBlock(
		p,
		QuorumCertFromProto(block.GetQC()),
		block.GetCommands(),
		hotstuff.View(block.GetView()),
		hotstuff.ID(block.GetProposer()),
	)
	b.SetTimestamp(block.Timestamp.AsTime())
	return b
}

// TimeoutMsgFromProto converts a TimeoutMsg to a hotstuff.TimeoutMsg.
func TimeoutMsgFromProto(m *TimeoutMsg) hotstuff.TimeoutMsg {
	timeoutMsg := hotstuff.TimeoutMsg{
		View:          hotstuff.View(m.GetView()),
		SyncInfo:      SyncInfoFromProto(m.GetSyncInfo()),
		ViewSignature: QuorumSignatureFromProto(m.GetViewSig()),
	}
	if m.GetMsgSig() != nil {
		timeoutMsg.MsgSignature = QuorumSignatureFromProto(m.GetMsgSig())
	}
	return timeoutMsg
}

// TimeoutMsgToProto converts a hotstuff.TimeoutMsg to a TimeoutMsg.
func TimeoutMsgToProto(timeoutMsg hotstuff.TimeoutMsg) *TimeoutMsg {
	tm := &TimeoutMsg{
		View:     uint64(timeoutMsg.View),
		SyncInfo: SyncInfoToProto(timeoutMsg.SyncInfo),
		ViewSig:  QuorumSignatureToProto(timeoutMsg.ViewSignature),
	}
	if timeoutMsg.MsgSignature != nil {
		tm.MsgSig = QuorumSignatureToProto(timeoutMsg.MsgSignature)
	}
	return tm
}

// TimeoutCertFromProto converts a TimeoutCert to a hotstuff.TimeoutCert.
func TimeoutCertFromProto(m *TimeoutCert) hotstuff.TimeoutCert {
	return hotstuff.NewTimeoutCert(QuorumSignatureFromProto(m.GetSig()), hotstuff.View(m.GetView()))
}

// TimeoutCertToProto converts a hotstuff.TimeoutCert to a TimeoutCert.
func TimeoutCertToProto(timeoutCert hotstuff.TimeoutCert) *TimeoutCert {
	return &TimeoutCert{
		View: uint64(timeoutCert.View()),
		Sig:  QuorumSignatureToProto(timeoutCert.Signature()),
	}
}

// AggregateQCFromProto converts an AggQC to a hotstuff.AggregateQC.
func AggregateQCFromProto(m *AggQC) hotstuff.AggregateQC {
	qcs := make(map[hotstuff.ID]hotstuff.QuorumCert)
	for id, pQC := range m.GetQCs() {
		qcs[hotstuff.ID(id)] = QuorumCertFromProto(pQC)
	}
	return hotstuff.NewAggregateQC(qcs, QuorumSignatureFromProto(m.GetSig()), hotstuff.View(m.GetView()))
}

// AggregateQCToProto converts a hotstuff.AggregateQC to an AggQC.
func AggregateQCToProto(aggQC hotstuff.AggregateQC) *AggQC {
	pQCs := make(map[uint32]*QuorumCert, len(aggQC.QCs()))
	for id, qc := range aggQC.QCs() {
		pQCs[uint32(id)] = QuorumCertToProto(qc)
	}
	return &AggQC{QCs: pQCs, Sig: QuorumSignatureToProto(aggQC.Sig()), View: uint64(aggQC.View())}
}

// SyncInfoFromProto converts a SyncInfo message to a hotstuff.SyncInfo.
func SyncInfoFromProto(m *SyncInfo) hotstuff.SyncInfo {
	si := hotstuff.NewSyncInfo()
	if qc := m.GetQC(); qc != nil {
		si = si.WithQC(QuorumCertFromProto(qc))
	}
	if tc := m.GetTC(); tc != nil {
		si = si.WithTC(TimeoutCertFromProto(tc))
	}
	if aggQC := m.GetAggQC(); aggQC != nil {
		si = si.WithAggQC(AggregateQCFromProto(aggQC))
	}
	return si
}

// SyncInfoToProto converts a hotstuff.SyncInfo to a SyncInfo message.
func SyncInfoToProto(syncInfo hotstuff.SyncInfo) *SyncInfo {
	m := &SyncInfo{}
	if qc, ok := syncInfo.QC(); ok {
		m.QC = QuorumCertToProto(qc)
	}
	if tc, ok := syncInfo.TC(); ok {
		m.TC = TimeoutCertToProto(tc)
	}
	if aggQC, ok := syncInfo.AggQC(); ok {
		m.AggQC = AggregateQCToProto(aggQC)
	}
	return m
}
