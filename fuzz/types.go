package fuzz

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/protobuf/proto"
)

// extractProtoMsg extracts the proto message from the FuzzMsg's oneof field.
func extractProtoMsg(fuzzMsg *FuzzMsg) (m proto.Message) {
	switch msg := fuzzMsg.Message.(type) {
	case *FuzzMsg_ProposeMsg:
		return msg.ProposeMsg
	case *FuzzMsg_TimeoutMsg:
		return msg.TimeoutMsg
	case *FuzzMsg_VoteMsg:
		return msg.VoteMsg
	case *FuzzMsg_NewViewMsg:
		return msg.NewViewMsg
	default:
		panic(fmt.Errorf("unknown message type: %T", msg))
	}
}

// fuzzMsgToHotStuffMsg returns a hotstuff message extracted from a FuzzMsg encapsulating a proto message.
func fuzzMsgToHotStuffMsg(fuzzMsg *FuzzMsg) any {
	switch msg := fuzzMsg.Message.(type) {
	case *FuzzMsg_ProposeMsg:
		proposeMsg := hotstuffpb.ProposalFromProto(msg.ProposeMsg.Proposal)
		proposeMsg.ID = hotstuff.ID(msg.ProposeMsg.ID)
		return proposeMsg

	case *FuzzMsg_TimeoutMsg:
		timeoutMsg := hotstuffpb.TimeoutMsgFromProto(msg.TimeoutMsg.TimeoutMsg)
		timeoutMsg.ID = hotstuff.ID(msg.TimeoutMsg.ID)
		return timeoutMsg

	case *FuzzMsg_VoteMsg:
		voteMsg := hotstuff.VoteMsg{}
		voteMsg.PartialCert = hotstuffpb.PartialCertFromProto(msg.VoteMsg.PartialCert)
		voteMsg.ID = hotstuff.ID(msg.VoteMsg.ID)
		voteMsg.Deferred = msg.VoteMsg.Deferred
		return voteMsg

	case *FuzzMsg_NewViewMsg:
		newViewMsg := hotstuff.NewViewMsg{}
		newViewMsg.SyncInfo = hotstuffpb.SyncInfoFromProto(msg.NewViewMsg.SyncInfo)
		newViewMsg.ID = hotstuff.ID(msg.NewViewMsg.ID)
		return newViewMsg

	default:
		panic(fmt.Errorf("unknown message type: %T", msg))
	}
}
