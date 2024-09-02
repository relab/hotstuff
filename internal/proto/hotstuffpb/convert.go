// Package hotstuffpb contains protocol buffers message types and conversion functions for the HotStuff protocol.
package hotstuffpb

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/eddsa"
)

// QuorumSignatureToProto converts a threshold signature to a protocol buffers message.
func QuorumSignatureToProto(sig hotstuff.QuorumSignature) *QuorumSignature {
	signature := &QuorumSignature{}
	switch ms := sig.(type) {
	case crypto.Multi[*ecdsa.Signature]:
		sigs := make([]*ECDSASignature, 0, sig.Participants().Len())
		for _, s := range ms {
			sigs = append(sigs, &ECDSASignature{
				Signer: uint32(s.Signer()),
				R:      s.R().Bytes(),
				S:      s.S().Bytes(),
			})
		}
		signature.Sig = &QuorumSignature_ECDSASigs{ECDSASigs: &ECDSAMultiSignature{
			Sigs: sigs,
		}}

	case crypto.Multi[*eddsa.Signature]:
		sigs := make([]*EDDSASignature, 0, sig.Participants().Len())
		for _, s := range ms {
			sigs = append(sigs, &EDDSASignature{Signer: uint32(s.Signer()), Sig: s.ToBytes()})
		}
		signature.Sig = &QuorumSignature_EDDSASigs{EDDSASigs: &EDDSAMultiSignature{
			Sigs: sigs,
		}}

	case *bls12.AggregateSignature:
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
		sigs := make([]*ecdsa.Signature, len(signature.GetSigs()))
		for i, sig := range signature.GetSigs() {
			r := new(big.Int)
			r.SetBytes(sig.GetR())
			s := new(big.Int)
			s.SetBytes(sig.GetS())
			sigs[i] = ecdsa.RestoreSignature(r, s, hotstuff.ID(sig.GetSigner()))
		}
		return crypto.Restore(sigs)
	}
	if signature := sig.GetEDDSASigs(); signature != nil {
		sigs := make([]*eddsa.Signature, len(signature.GetSigs()))
		for i, sig := range signature.GetSigs() {
			sigs[i] = eddsa.RestoreSignature(sig.Sig, hotstuff.ID(sig.GetSigner()))
		}
		return crypto.Restore(sigs)
	}
	if signature := sig.GetBLS12Sig(); signature != nil {
		aggSig, err := bls12.RestoreAggregateSignature(signature.GetSig(), crypto.BitfieldFromBytes(signature.GetParticipants()))
		if err != nil {
			return nil
		}
		return aggSig
	}
	return nil
}

// PartialCertToProto converts a consensus.PartialCert to a hotstuffpb.PartialCert.
func PartialCertToProto(cert hotstuff.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  QuorumSignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
}

// PartialCertFromProto converts a hotstuffpb.PartialCert to an ecdsa.PartialCert.
func PartialCertFromProto(cert *PartialCert) hotstuff.PartialCert {
	var h hotstuff.Hash
	copy(h[:], cert.GetHash())
	return hotstuff.NewPartialCert(QuorumSignatureFromProto(cert.GetSig()), h)
}

// QuorumCertToProto converts a consensus.QuorumCert to a hotstuffpb.QuorumCert.
func QuorumCertToProto(qc hotstuff.QuorumCert) *QuorumCert {
	hash := qc.BlockHash()
	return &QuorumCert{
		Sig:  QuorumSignatureToProto(qc.Signature()),
		Hash: hash[:],
		View: uint64(qc.View()),
	}
}

// QuorumCertFromProto converts a hotstuffpb.QuorumCert to an ecdsa.QuorumCert.
func QuorumCertFromProto(qc *QuorumCert) hotstuff.QuorumCert {
	var h hotstuff.Hash
	copy(h[:], qc.GetHash())
	return hotstuff.NewQuorumCert(QuorumSignatureFromProto(qc.GetSig()), hotstuff.View(qc.GetView()), h)
}

// ProposalToProto converts a ProposeMsg to a protobuf message.
func ProposalToProto(proposal hotstuff.ProposeMsg) *Proposal {
	p := &Proposal{
		Block: BlockToProto(proposal.Block),
	}
	if proposal.AggregateQC != nil {
		p.AggQC = AggregateQCToProto(*proposal.AggregateQC)
	}
	return p
}

// ProposalFromProto converts a protobuf message to a ProposeMsg.
func ProposalFromProto(p *Proposal) (proposal hotstuff.ProposeMsg) {
	proposal.Block = BlockFromProto(p.GetBlock())
	if p.GetAggQC() != nil {
		aggQC := AggregateQCFromProto(p.GetAggQC())
		proposal.AggregateQC = &aggQC
	}
	return
}

// BlockToProto converts a consensus.Block to a hotstuffpb.Block.
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

// BlockFromProto converts a hotstuffpb.Block to a consensus.Block.
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

// TimeoutMsgFromProto converts a TimeoutMsg proto to the hotstuff type.
func TimeoutMsgFromProto(m *TimeoutMsg) hotstuff.TimeoutMsg {
	timeoutMsg := hotstuff.TimeoutMsg{
		View:          hotstuff.View(m.GetView()),
		SyncInfo:      SyncInfoFromProto(m.GetSyncInfo()),
		ViewSignature: QuorumSignatureFromProto(m.GetViewSig()),
	}
	if m.GetViewSig() != nil {
		timeoutMsg.MsgSignature = QuorumSignatureFromProto(m.GetMsgSig())
	}
	return timeoutMsg
}

// TimeoutMsgToProto converts a TimeoutMsg to the protobuf type.
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

// TimeoutCertFromProto converts a timeout certificate from the protobuf type to the hotstuff type.
func TimeoutCertFromProto(m *TimeoutCert) hotstuff.TimeoutCert {
	return hotstuff.NewTimeoutCert(QuorumSignatureFromProto(m.GetSig()), hotstuff.View(m.GetView()))
}

// TimeoutCertToProto converts a timeout certificate from the hotstuff type to the protobuf type.
func TimeoutCertToProto(timeoutCert hotstuff.TimeoutCert) *TimeoutCert {
	return &TimeoutCert{
		View: uint64(timeoutCert.View()),
		Sig:  QuorumSignatureToProto(timeoutCert.Signature()),
	}
}

// AggregateQCFromProto converts an AggregateQC from the protobuf type to the hotstuff type.
func AggregateQCFromProto(m *AggQC) hotstuff.AggregateQC {
	qcs := make(map[hotstuff.ID]hotstuff.QuorumCert)
	for id, pQC := range m.GetQCs() {
		qcs[hotstuff.ID(id)] = QuorumCertFromProto(pQC)
	}
	return hotstuff.NewAggregateQC(qcs, QuorumSignatureFromProto(m.GetSig()), hotstuff.View(m.GetView()))
}

// AggregateQCToProto converts an AggregateQC from the hotstuff type to the protobuf type.
func AggregateQCToProto(aggQC hotstuff.AggregateQC) *AggQC {
	pQCs := make(map[uint32]*QuorumCert, len(aggQC.QCs()))
	for id, qc := range aggQC.QCs() {
		pQCs[uint32(id)] = QuorumCertToProto(qc)
	}
	return &AggQC{QCs: pQCs, Sig: QuorumSignatureToProto(aggQC.Sig()), View: uint64(aggQC.View())}
}

// SyncInfoFromProto converts a SyncInfo struct from the protobuf type to the hotstuff type.
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

// SyncInfoToProto converts a SyncInfo struct from the hotstuff type to the protobuf type.
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
