package hotstuffpb

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

// SignatureToProto converts a consensus.Signature to a hotstuffpb.Signature.
func SignatureToProto(sig consensus.Signature) *Signature {
	signature := &Signature{}
	switch s := sig.(type) {
	case *ecdsa.Signature:
		signature.Sig = &Signature_ECDSASig{ECDSASig: &ECDSASignature{
			Signer: uint32(s.Signer()),
			R:      s.R().Bytes(),
			S:      s.S().Bytes(),
		}}
	case *bls12.Signature:
		signature.Sig = &Signature_BLS12Sig{BLS12Sig: &BLS12Signature{
			Sig: s.ToBytes(),
		}}
	}
	return signature
}

// SignatureFromProto converts a hotstuffpb.Signature to an ecdsa.Signature.
func SignatureFromProto(sig *Signature) consensus.Signature {
	if signature := sig.GetECDSASig(); signature != nil {
		r := new(big.Int)
		r.SetBytes(signature.GetR())
		s := new(big.Int)
		s.SetBytes(signature.GetS())
		return ecdsa.RestoreSignature(r, s, hotstuff.ID(signature.GetSigner()))
	}
	if signature := sig.GetBLS12Sig(); signature != nil {
		s := &bls12.Signature{}
		err := s.FromBytes(signature.GetSig())
		if err != nil {
			return nil
		}
		return s
	}
	return nil
}

// ThresholdSignatureToProto converts a threshold signature to a protocol buffers message.
func ThresholdSignatureToProto(sig consensus.ThresholdSignature) *ThresholdSignature {
	signature := &ThresholdSignature{}
	switch s := sig.(type) {
	case ecdsa.ThresholdSignature:
		sigs := make([]*ECDSASignature, 0, len(s))
		for _, p := range s {
			sigs = append(sigs, &ECDSASignature{
				Signer: uint32(p.Signer()),
				R:      p.R().Bytes(),
				S:      p.S().Bytes(),
			})
		}
		signature.AggSig = &ThresholdSignature_ECDSASigs{ECDSASigs: &ECDSAThresholdSignature{
			Sigs: sigs,
		}}
	case *bls12.AggregateSignature:
		signature.AggSig = &ThresholdSignature_BLS12Sig{BLS12Sig: &BLS12AggregateSignature{
			Sig:          s.ToBytes(),
			Participants: s.Bitfield().Bytes(),
		}}
	}
	return signature
}

// ThresholdSignatureFromProto converts a protocol buffers message to a threshold signature.
func ThresholdSignatureFromProto(sig *ThresholdSignature) consensus.ThresholdSignature {
	if signature := sig.GetECDSASigs(); signature != nil {
		sigs := make([]*ecdsa.Signature, len(signature.GetSigs()))
		for i, sig := range signature.GetSigs() {
			r := new(big.Int)
			r.SetBytes(sig.GetR())
			s := new(big.Int)
			s.SetBytes(sig.GetS())
			sigs[i] = ecdsa.RestoreSignature(r, s, hotstuff.ID(sig.GetSigner()))
		}
		return ecdsa.RestoreThresholdSignature(sigs)
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

// PartialCertToProto converts a consensus.PartialCert to a hotstuffpb.Partialcert.
func PartialCertToProto(cert consensus.PartialCert) *PartialCert {
	hash := cert.BlockHash()
	return &PartialCert{
		Sig:  SignatureToProto(cert.Signature()),
		Hash: hash[:],
	}
}

// PartialCertFromProto converts a hotstuffpb.PartialCert to an ecdsa.PartialCert.
func PartialCertFromProto(cert *PartialCert) consensus.PartialCert {
	var h consensus.Hash
	copy(h[:], cert.GetHash())
	return consensus.NewPartialCert(SignatureFromProto(cert.GetSig()), h)
}

// QuorumCertToProto converts a consensus.QuorumCert to a hotstuffpb.QuorumCert.
func QuorumCertToProto(qc consensus.QuorumCert) *QuorumCert {
	hash := qc.BlockHash()
	return &QuorumCert{
		Sig:  ThresholdSignatureToProto(qc.Signature()),
		Hash: hash[:],
		View: uint64(qc.View()),
	}
}

// QuorumCertFromProto converts a hotstuffpb.QuorumCert to an ecdsa.QuorumCert.
func QuorumCertFromProto(qc *QuorumCert) consensus.QuorumCert {
	var h consensus.Hash
	copy(h[:], qc.GetHash())
	return consensus.NewQuorumCert(ThresholdSignatureFromProto(qc.GetSig()), consensus.View(qc.GetView()), h)
}

// ProposalToProto converts a ProposeMsg to a protobuf message.
func ProposalToProto(proposal consensus.ProposeMsg) *Proposal {
	p := &Proposal{
		Block: BlockToProto(proposal.Block),
	}
	if proposal.AggregateQC != nil {
		p.AggQC = AggregateQCToProto(*proposal.AggregateQC)
	}
	return p
}

// ProposalFromProto converts a protobuf message to a ProposeMsg.
func ProposalFromProto(p *Proposal) (proposal consensus.ProposeMsg) {
	proposal.Block = BlockFromProto(p.GetBlock())
	if p.GetAggQC() != nil {
		aggQC := AggregateQCFromProto(p.GetAggQC())
		proposal.AggregateQC = &aggQC
	}
	return
}

// BlockToProto converts a consensus.Block to a hotstuffpb.Block.
func BlockToProto(block *consensus.Block) *Block {
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
func BlockFromProto(block *Block) *consensus.Block {
	var p consensus.Hash
	copy(p[:], block.GetParent())
	return consensus.NewBlock(
		p,
		QuorumCertFromProto(block.GetQC()),
		consensus.Command(block.GetCommand()),
		consensus.View(block.GetView()),
		hotstuff.ID(block.GetProposer()),
	)
}

// TimeoutMsgFromProto converts a TimeoutMsg proto to the hotstuff type.
func TimeoutMsgFromProto(m *TimeoutMsg) consensus.TimeoutMsg {
	timeoutMsg := consensus.TimeoutMsg{
		View:          consensus.View(m.GetView()),
		SyncInfo:      SyncInfoFromProto(m.GetSyncInfo()),
		ViewSignature: SignatureFromProto(m.GetViewSig()),
	}
	if m.GetViewSig() != nil {
		timeoutMsg.MsgSignature = SignatureFromProto(m.GetMsgSig())
	}
	return timeoutMsg
}

// TimeoutMsgToProto converts a TimeoutMsg to the protobuf type.
func TimeoutMsgToProto(timeoutMsg consensus.TimeoutMsg) *TimeoutMsg {
	tm := &TimeoutMsg{
		View:     uint64(timeoutMsg.View),
		SyncInfo: SyncInfoToProto(timeoutMsg.SyncInfo),
		ViewSig:  SignatureToProto(timeoutMsg.ViewSignature),
	}
	if timeoutMsg.MsgSignature != nil {
		tm.MsgSig = SignatureToProto(timeoutMsg.MsgSignature)
	}
	return tm
}

// TimeoutCertFromProto converts a timeout certificate from the protobuf type to the hotstuff type.
func TimeoutCertFromProto(m *TimeoutCert) consensus.TimeoutCert {
	return consensus.NewTimeoutCert(ThresholdSignatureFromProto(m.GetSig()), consensus.View(m.GetView()))
}

// TimeoutCertToProto converts a timeout certificate from the hotstuff type to the protobuf type.
func TimeoutCertToProto(timeoutCert consensus.TimeoutCert) *TimeoutCert {
	return &TimeoutCert{
		View: uint64(timeoutCert.View()),
		Sig:  ThresholdSignatureToProto(timeoutCert.Signature()),
	}
}

// AggregateQCFromProto converts an AggregateQC from the protobuf type to the hotstuff type.
func AggregateQCFromProto(m *AggQC) consensus.AggregateQC {
	qcs := make(map[hotstuff.ID]consensus.QuorumCert)
	for id, pQC := range m.GetQCs() {
		qcs[hotstuff.ID(id)] = QuorumCertFromProto(pQC)
	}
	return consensus.NewAggregateQC(qcs, ThresholdSignatureFromProto(m.GetSig()), consensus.View(m.GetView()))
}

// AggregateQCToProto converts an AggregateQC from the hotstuff type to the protobuf type.
func AggregateQCToProto(aggQC consensus.AggregateQC) *AggQC {
	pQCs := make(map[uint32]*QuorumCert, len(aggQC.QCs()))
	for id, qc := range aggQC.QCs() {
		pQCs[uint32(id)] = QuorumCertToProto(qc)
	}
	return &AggQC{QCs: pQCs, Sig: ThresholdSignatureToProto(aggQC.Sig()), View: uint64(aggQC.View())}
}

// SyncInfoFromProto converts a SyncInfo struct from the protobuf type to the hotstuff type.
func SyncInfoFromProto(m *SyncInfo) consensus.SyncInfo {
	si := consensus.NewSyncInfo()
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
func SyncInfoToProto(syncInfo consensus.SyncInfo) *SyncInfo {
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
