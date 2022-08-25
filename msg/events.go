package msg

import (
	"fmt"

	"github.com/relab/hotstuff"
	"google.golang.org/protobuf/proto"
)

// ProposeMsg is broadcast when a leader makes a proposal.
//type ProposeMsg struct {
//	Block       *Block       // The block that is proposed.
//	AggregateQC *AggregateQC // Optional AggregateQC
//}

// VoteMsg is sent to the leader by replicas voting on a proposal.
//type VoteMsg struct {
//	ID          hotstuff.ID  // the ID of the replica who sent the message.
//	PartialCert *PartialCert // The partial certificate.
//	Deferred    bool
//}

// TimeoutMsg is broadcast whenever a replica has a local timeout.
// type TimeoutMsg struct {
// 	ID            hotstuff.ID     // The ID of the replica who sent the message.
// 	View          View            // The view that the replica wants to enter.
// 	ViewSignature QuorumSignature // A signature of the view
// 	MsgSignature  QuorumSignature // A signature of the view, QC.BlockHash, and the replica ID
// 	SyncInfo      SyncInfo        // The highest QC/TC known to the sender.
// }

func NewTimeoutMsg(id hotstuff.ID, view View, syncInfo *SyncInfo, sig *Signature) *TimeoutMsg {
	return &TimeoutMsg{
		View:     uint64(view),
		SyncInfo: syncInfo,
		ViewSig:  sig,
		ID:       uint32(id),
	}
}

// // Hash returns a hash of the timeout message.
// func (timeout TimeoutMsg) Hash() Hash {
// 	var h Hash
// 	hash := sha256.New()
// 	hash.Write(timeout.View.ToBytes())
// 	if qc, ok := timeout.SyncInfo.QC(); ok {
// 		h := qc.BlockHash()
// 		hash.Write(h[:])
// 	}
// 	hash.Sum(h[:0])
// 	return h
// }

func (timeout *TimeoutMsg) TString() string {
	return fmt.Sprintf("TimeoutMsg{ ID: %d, View: %d, SyncInfo: %v }", timeout.ID, timeout.View, timeout.SyncInfo)
}

// ToBytes returns a byte form of the timeout message.
func (timeout *TimeoutMsg) ToBytes() []byte {
	if timeout == nil {
		return nil
	}
	y, err := proto.Marshal(timeout)
	if err != nil {
		return nil
	}
	return y
}

// NewViewMsg is sent to the leader whenever a replica decides to advance to the next view.
// It contains the highest QC or TC known to the replica.
//type NewViewMsg struct {
//	ID       hotstuff.ID // The ID of the replica who sent the message.
//	SyncInfo *SyncInfo   // The highest QC / TC.
//}

// CommitEvent is raised whenever a block is committed,
// and includes the number of client commands that were executed.
type CommitEvent struct {
	Commands int
}
