package fuzz

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

type FuzzMsgInterface interface {
	ToMsg() any
	ToString(int) string
	String() string
}

type AlmostFuzzMsg interface {
	Msg() FuzzMsgInterface
}

func (proposeFuzzMsg *FuzzMsg_ProposeMsg) Msg() FuzzMsgInterface {
	return proposeFuzzMsg.ProposeMsg
}

func (timeoutFuzzMsg *FuzzMsg_TimeoutMsg) Msg() FuzzMsgInterface {
	return timeoutFuzzMsg.TimeoutMsg
}

func (newViewFuzzMsg *FuzzMsg_NewViewMsg) Msg() FuzzMsgInterface {
	return newViewFuzzMsg.NewViewMsg
}

func (voteFuzzMsg *FuzzMsg_VoteMsg) Msg() FuzzMsgInterface {
	return voteFuzzMsg.VoteMsg
}

func (fuzzMsg *FuzzMsg) Msg() FuzzMsgInterface {
	return fuzzMsg.Message.(AlmostFuzzMsg).Msg()
}

func (proposeFuzzMsg *ProposeMsg) ToMsg() any {
	proposeMsg := hotstuffpb.ProposalFromProto(proposeFuzzMsg.Proposal)
	proposeMsg.ID = hotstuff.ID(proposeFuzzMsg.ID)
	return proposeMsg
}

func (timeoutFuzzMsg *TimeoutMsg) ToMsg() any {
	timeoutMsg := hotstuffpb.TimeoutMsgFromProto(timeoutFuzzMsg.TimeoutMsg)
	timeoutMsg.ID = hotstuff.ID(timeoutFuzzMsg.ID)
	return timeoutMsg
}

func (voteFuzzMsg *VoteMsg) ToMsg() any {
	voteMsg := hotstuff.VoteMsg{}
	voteMsg.PartialCert = hotstuffpb.PartialCertFromProto(voteFuzzMsg.PartialCert)
	voteMsg.ID = hotstuff.ID(voteFuzzMsg.ID)
	voteMsg.Deferred = voteFuzzMsg.Deferred
	return voteMsg
}

func (newViewFuzzMsg *NewViewMsg) ToMsg() any {
	newViewMsg := hotstuff.NewViewMsg{}
	newViewMsg.SyncInfo = hotstuffpb.SyncInfoFromProto(newViewFuzzMsg.SyncInfo)
	newViewMsg.ID = hotstuff.ID(newViewFuzzMsg.ID)
	return newViewMsg
}
