package proto

import (
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

// SignatureToProto converts a hotstuff.Signature to a proto.Signature.
func SignatureToProto(sig hotstuff.Signature) *Signature {
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

// SignatureFromProto converts a proto.Signature to an ecdsa.Signature.
func SignatureFromProto(sig *Signature) hotstuff.Signature {
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
func ThresholdSignatureToProto(sig hotstuff.ThresholdSignature) *ThresholdSignature {
	signature := &ThresholdSignature{}
	switch s := sig.(type) {
	case *ecdsa.ThresholdSignature:
		sigs := make([]*ECDSASignature, len(s.Signatures()))
		for i, p := range s.Signatures() {
			sigs[i] = &ECDSASignature{
				Signer: uint32(p.Signer()),
				R:      p.R().Bytes(),
				S:      p.S().Bytes(),
			}
		}
		signature.AggSig = &ThresholdSignature_ECDSASigs{ECDSASigs: &ECDSAThresholdSignature{
			Sigs: sigs,
		}}
	case *bls12.AggregateSignature:
		participants := make([]uint32, 0, len(s.Participants()))
		for p := range s.Participants() {
			participants = append(participants, uint32(p))
		}
		signature.AggSig = &ThresholdSignature_BLS12Sig{BLS12Sig: &BLS12AggregateSignature{
			Sig:          s.ToBytes(),
			Participants: participants,
		}}
	}
	return signature
}

// ThresholdSignatureFromProto converts a protocol buffers message to a threshold signature.
func ThresholdSignatureFromProto(sig *ThresholdSignature) hotstuff.ThresholdSignature {
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
		participants := make(hotstuff.IDSet, len(signature.GetParticipants()))
		for _, participant := range signature.GetParticipants() {
			participants.Add(hotstuff.ID(participant))
		}
		aggSig, err := bls12.RestoreAggregateSignature(signature.GetSig(), participants)
		if err != nil {
			return nil
		}
		return aggSig
	}
	return nil
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
func PartialCertFromProto(cert *PartialCert) hotstuff.PartialCert {
	var h hotstuff.Hash
	copy(h[:], cert.GetHash())
	return hotstuff.NewPartialCert(SignatureFromProto(cert.GetSig()), h)
}

// QuorumCertToProto converts a hotstuff.QuorumCert to a proto.QuorumCert.
func QuorumCertToProto(qc hotstuff.QuorumCert) *QuorumCert {
	hash := qc.BlockHash()
	return &QuorumCert{
		Sig:  ThresholdSignatureToProto(qc.Signature()),
		Hash: hash[:],
	}
}

// QuorumCertFromProto converts a proto.QuorumCert to an ecdsa.QuorumCert.
func QuorumCertFromProto(qc *QuorumCert) hotstuff.QuorumCert {
	var h hotstuff.Hash
	copy(h[:], qc.GetHash())
	return hotstuff.NewQuorumCert(ThresholdSignatureFromProto(qc.GetSig()), h)
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

// TimeoutMsgFromProto converts a TimeoutMsg proto to the hotstuff type.
func TimeoutMsgFromProto(m *TimeoutMsg) hotstuff.TimeoutMsg {
	return hotstuff.TimeoutMsg{
		View:      hotstuff.View(m.GetView()),
		SyncInfo:  SyncInfoFromProto(m.GetSyncInfo()),
		Signature: SignatureFromProto(m.GetSig()),
	}
}

// TimeoutMsgToProto converts a TimeoutMsg to the protobuf type.
func TimeoutMsgToProto(timeoutMsg hotstuff.TimeoutMsg) *TimeoutMsg {
	return &TimeoutMsg{
		View:     uint64(timeoutMsg.View),
		SyncInfo: SyncInfoToProto(timeoutMsg.SyncInfo),
		Sig:      SignatureToProto(timeoutMsg.Signature),
	}
}

// TimeoutCertFromProto converts a timeout certificate from the protobuf type to the hotstuff type.
func TimeoutCertFromProto(m *TimeoutCert) hotstuff.TimeoutCert {
	return hotstuff.NewTimeoutCert(ThresholdSignatureFromProto(m.GetSig()), hotstuff.View(m.GetView()))
}

// TimeoutCertToProto converts a timeout certificate from the hotstuff type to the protobuf type.
func TimeoutCertToProto(timeoutCert hotstuff.TimeoutCert) *TimeoutCert {
	return &TimeoutCert{
		View: uint64(timeoutCert.View()),
		Sig:  ThresholdSignatureToProto(timeoutCert.Signature()),
	}
}

// SyncInfoFromProto converts a SyncInfo struct from the protobuf type to the hotstuff type.
func SyncInfoFromProto(m *SyncInfo) hotstuff.SyncInfo {
	if qc := m.GetQC(); qc != nil {
		return hotstuff.SyncInfoWithQC(QuorumCertFromProto(qc))
	}
	if tc := m.GetTC(); tc != nil {
		return hotstuff.SyncInfoWithTC(TimeoutCertFromProto(tc))
	}
	return hotstuff.SyncInfo{}
}

// SyncInfoToProto converts a SyncInfo struct from the hotstuff type to the protobuf type.
func SyncInfoToProto(syncInfo hotstuff.SyncInfo) *SyncInfo {
	m := &SyncInfo{}
	if qc, ok := syncInfo.QC(); ok {
		m.Certificate = &SyncInfo_QC{QuorumCertToProto(qc)}
	}
	if tc, ok := syncInfo.TC(); ok {
		m.Certificate = &SyncInfo_TC{TimeoutCertToProto(tc)}
	}
	return m
}
