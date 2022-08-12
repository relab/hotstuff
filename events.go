package hotstuff

import (
	"bytes"
	"fmt"
)

// ProposeMsg is broadcast when a leader makes a proposal.
type ProposeMsg struct {
	ID          ID           // The ID of the replica who sent the message.
	Block       *Block       // The block that is proposed.
	AggregateQC *AggregateQC // Optional AggregateQC
}

func (p ProposeMsg) String() string {
	return fmt.Sprintf("ID %d, %s, AggQC: %v", p.ID, p.Block, p.AggregateQC != nil)
}

// VoteMsg is sent to the leader by replicas voting on a proposal.
type VoteMsg struct {
	ID          ID          // the ID of the replica who sent the message.
	PartialCert PartialCert // The partial certificate.
	Deferred    bool
}

func (v VoteMsg) String() string {
	return fmt.Sprintf("ID %d", v.ID)
}

// TimeoutMsg is broadcast whenever a replica has a local timeout.
type TimeoutMsg struct {
	ID            ID              // The ID of the replica who sent the message.
	View          View            // The view that the replica wants to enter.
	ViewSignature QuorumSignature // A signature of the view
	MsgSignature  QuorumSignature // A signature of the view, QC.BlockHash, and the replica ID
	SyncInfo      SyncInfo        // The highest QC/TC known to the sender.
}

// ToBytes returns a byte form of the timeout message.
func (timeout TimeoutMsg) ToBytes() []byte {
	var b bytes.Buffer
	_, _ = b.Write(timeout.ID.ToBytes())
	_, _ = b.Write(timeout.View.ToBytes())
	if qc, ok := timeout.SyncInfo.QC(); ok {
		_, _ = b.Write(qc.ToBytes())
	}
	return b.Bytes()
}

func (timeout TimeoutMsg) String() string {
	return fmt.Sprintf("ID: %d, View: %d, SyncInfo: %v", timeout.ID, timeout.View, timeout.SyncInfo)
}

// NewViewMsg is sent to the leader whenever a replica decides to advance to the next view.
// It contains the highest QC or TC known to the replica.
type NewViewMsg struct {
	ID       ID       // The ID of the replica who sent the message.
	SyncInfo SyncInfo // The highest QC / TC.
}

// CommitEvent is raised whenever a block is committed,
// and includes the number of client commands that were executed.
type CommitEvent struct {
	Commands int
}
