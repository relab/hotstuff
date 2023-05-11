package fuzz

import (
	"fmt"

	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

func depthToTabs(depth int) (tabs string) {
	for i := 0; i < depth; i++ {
		tabs += "\t"
	}
	return
}

func (proposeFuzzMsg *ProposeMsg) ToString(depth int) string {
	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"fuzz.ProposeMsg{\n"+
			"%s\tID: %v\n"+
			"%s\tProposal: %v\n"+
			"%s}",
		tabs, proposeFuzzMsg.ID,
		tabs, ProposalToString(proposeFuzzMsg.Proposal, depth+1),
		tabs)
}

func (timeoutFuzzMsg *TimeoutMsg) ToString(depth int) string {

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"fuzz.TimeoutMsg{\n"+
			"%s\tID: %v\n"+
			"%s\tTimeoutMsg: %v\n"+
			"%s}",
		tabs, timeoutFuzzMsg.ID,
		tabs, TimeoutMsgToString(timeoutFuzzMsg.TimeoutMsg, depth+1),
		tabs)
}

func (voteFuzzMsg *VoteMsg) ToString(depth int) string {
	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"fuzz.VoteMsg{\n"+
			"%s\tID: %v\n"+
			"%s\tDeffered: %v\n"+
			"%s\tPartialCert: %v\n"+
			"%s}",
		tabs, voteFuzzMsg.ID,
		tabs, voteFuzzMsg.Deferred,
		tabs, PartialCertToString(voteFuzzMsg.PartialCert, depth+1),
		tabs)
}

func (newViewFuzzMsg *NewViewMsg) ToString(depth int) string {

	if newViewFuzzMsg == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"fuzz.NewViewMsg{\n"+
			"%s\tID: %v\n"+
			"%s\tSyncInfo: %v\n"+
			"%s}",
		tabs, newViewFuzzMsg.ID,
		tabs, SyncInfoToString(newViewFuzzMsg.SyncInfo, depth+1),
		tabs)
}

func ProposalToString(proposal *hotstuffpb.Proposal, depth int) string {

	if proposal == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"hotstuffpb.Proposal{\n"+
			"%s\tBlock: %v\n"+
			"%s\tAggQC: %v\n"+
			"%s}",
		tabs, BlockToString(proposal.Block, depth+1),
		tabs, AggQCToString(proposal.AggQC, depth+1),
		tabs)
}

func BlockToString(block *hotstuffpb.Block, depth int) string {

	if block == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.Block{\n"+
			"%s\tParent: %v\n"+
			"%s\tQC: %v\n"+
			"%s\tView: %v\n"+
			"%s\tCommand: %v\n"+
			"%s\tProposer: %v\n"+
			"%s}",
		tabs, block.Parent,
		tabs, QuorumCertToString(block.QC, depth+1),
		tabs, block.View,
		tabs, block.Command,
		tabs, block.Proposer,
		tabs)
}

func TimeoutMsgToString(timeoutMsg *hotstuffpb.TimeoutMsg, depth int) string {

	if timeoutMsg == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"hotstuffpb.TimeoutMsg{\n"+
			"%s\tView: %v\n"+
			"%s\tSyncInfo: %v\n"+
			"%s\tViewSig: %v\n"+
			"%s\tMsgSig: %v\n"+
			"%s}",
		tabs, timeoutMsg.View,
		tabs, SyncInfoPtrToString(timeoutMsg.SyncInfo, depth+1),
		tabs, QuorumSignatureToString(timeoutMsg.ViewSig, depth+1),
		tabs, QuorumSignatureToString(timeoutMsg.MsgSig, depth+1),
		tabs)
}

func SyncInfoPtrToString(object *hotstuffpb.SyncInfo, depth int) string {
	if object == nil {
		return "nil"
	}

	return "&" + SyncInfoToString(object, depth)
}

func SyncInfoToString(syncInfo *hotstuffpb.SyncInfo, depth int) string {

	if syncInfo == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"hotstuffpb.SyncInfo{\n"+
			"%s\tQC: %v\n"+
			"%s\tTC: %v\n"+
			"%s\tAggQC: %v\n"+
			"%s}",
		tabs, QuorumCertToString(syncInfo.QC, depth+1),
		tabs, TimeoutCertToString(syncInfo.TC, depth+1),
		tabs, AggQCToString(syncInfo.AggQC, depth+1),
		tabs)
}

func QuorumCertToString(qc *hotstuffpb.QuorumCert, depth int) string {

	if qc == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.QuorumCert{\n"+
			"%s\tSig: %v\n"+
			"%s\tView: %v\n"+
			"%s\tHash: %v\n"+
			"%s}",
		tabs, QuorumSignatureToString(qc.Sig, depth+1),
		tabs, qc.View,
		tabs, qc.Hash,
		tabs)
}

func TimeoutCertToString(tc *hotstuffpb.TimeoutCert, depth int) string {

	if tc == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.TimeoutCert{\n"+
			"%s\tSig: %v\n"+
			"%s\tView: %v\n"+
			"%s}",
		tabs, QuorumSignatureToString(tc.Sig, depth+1),
		tabs, tc.View,
		tabs)
}

func AggQCToString(aggQC *hotstuffpb.AggQC, depth int) string {

	if aggQC == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	QCsString := "map[uint32]*hotstuffpb.QuorumCert{\n"

	for key, value := range aggQC.QCs {
		QCsString += fmt.Sprintf("%v\t\t%v: %v\n", tabs, key, QuorumCertToString(value, depth+2))
	}

	QCsString += tabs + "\t"

	return fmt.Sprintf(
		"&hotstuffpb.AggQC{\n"+
			"%s\tQCs: %v\n"+
			"%s\tSig: %v\n"+
			"%s\tView: %v\n"+
			"%s}",
		tabs, QCsString,
		tabs, QuorumSignatureToString(aggQC.Sig, depth+1),
		tabs, aggQC.View,
		tabs)
}

func PartialCertToString(partialCert *hotstuffpb.PartialCert, depth int) string {

	if partialCert == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"hotstuffpb.PartialCert{\n"+
			"%s\tSig: %v\n"+
			"%s\tHash: %v\n"+
			"%s}",
		tabs, QuorumSignatureToString(partialCert.Sig, depth+1),
		tabs, partialCert.Hash,
		tabs)
}

func QuorumSignatureToString(quorumSignature *hotstuffpb.QuorumSignature, depth int) string {

	if quorumSignature == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	sigString := ""
	if quorumSignature.Sig == nil {
		sigString = "nil"
	} else {
		switch quorumSignature.Sig.(type) {
		case *hotstuffpb.QuorumSignature_ECDSASigs:
			sigString = QuorumSignature_ECDSASigsToString(quorumSignature.Sig.(*hotstuffpb.QuorumSignature_ECDSASigs), depth+1)
		case *hotstuffpb.QuorumSignature_BLS12Sig:
			sigString = QuorumSignature_BLS12SigToString(quorumSignature.Sig.(*hotstuffpb.QuorumSignature_BLS12Sig), depth+1)
		}
	}

	return fmt.Sprintf(
		"&hotstuffpb.QuorumSignature{\n"+
			"%s\tSig: %s\n"+
			"%s}",
		tabs, sigString,
		tabs)
}

func QuorumSignature_ECDSASigsToString(ECDASigs *hotstuffpb.QuorumSignature_ECDSASigs, depth int) string {

	if ECDASigs == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.QuorumSignature_ECDSASigs{\n"+
			"%s\tECDASigs: %s\n"+
			"%s},",
		tabs, ECDSAMultiSignatureToString(ECDASigs.ECDSASigs, depth+1),
		tabs)
}

func ECDSAMultiSignatureToString(ECDASigs *hotstuffpb.ECDSAMultiSignature, depth int) string {

	if ECDASigs == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	sigsString := "[]*ECDASignature{\n"

	for _, sig := range ECDASigs.Sigs {
		sigsString += fmt.Sprintf("%s\t\t%s\n", tabs, ECDASigToString(sig, depth+2))
	}

	sigsString += tabs + "\t}"

	return fmt.Sprintf(
		"&hotstuffpb.ECDSAMultiSignature{\n"+
			"%s\tECDASigs: %s\n"+
			"%s}",
		tabs, sigsString,
		tabs)
}

func ECDASigToString(sig *hotstuffpb.ECDSASignature, depth int) string {

	if sig == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.ECDSASignature{\n"+
			"%s\tSigner: %v\n"+
			"%s\tR: %v\n"+
			"%s\tS: %v\n"+
			"%s}",
		tabs, sig.Signer,
		tabs, sig.R,
		tabs, sig.S,
		tabs)
}

func QuorumSignature_BLS12SigToString(BLS12Sig *hotstuffpb.QuorumSignature_BLS12Sig, depth int) string {

	if BLS12Sig == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.QuorumSignature_BLS12Sig{\n"+
			"%s\tBLS12Sig: %s\n"+
			"%s},",
		tabs, BLS12AggregateSignatureToString(BLS12Sig.BLS12Sig, depth+1),
		tabs)
}

func BLS12AggregateSignatureToString(BLS12Sig *hotstuffpb.BLS12AggregateSignature, depth int) string {

	if BLS12Sig == nil {
		return "nil"
	}

	tabs := depthToTabs(depth)

	return fmt.Sprintf(
		"&hotstuffpb.BLS12AggregateSignature{\n"+
			"%s\tSig: %v\n"+
			"%s\tParticipants: %v\n"+
			"%s},",
		tabs, BLS12Sig.Sig,
		tabs, BLS12Sig.Participants,
		tabs)
}
