package consensus

import (
	"crypto/sha256"
	"fmt"

	"github.com/relab/hotstuff"
)

// ProposeMsg is broadcast when a leader makes a proposal.
type ProposeMsg struct {
	ID          hotstuff.ID  // The ID of the replica who sent the message.
	Block       *Block       // The block that is proposed.
	AggregateQC *AggregateQC // Optional AggregateQC
}

// VoteMsg is sent to the leader by replicas voting on a proposal.
type VoteMsg struct {
	ID          hotstuff.ID // the ID of the replica who sent the message.
	PartialCert PartialCert // The partial certificate.
	Deferred    bool
}

// TimeoutMsg is broadcast whenever a replica has a local timeout.
type TimeoutMsg struct {
	ID            hotstuff.ID // The ID of the replica who sent the message.
	View          View        // The view that the replica wants to enter.
	ViewSignature Signature   // A signature of the view
	MsgSignature  Signature   // A signature of the view, QC.BlockHash, and the replica ID
	SyncInfo      SyncInfo    // The highest QC/TC known to the sender.
}

// Hash returns a hash of the timeout message.
func (timeout TimeoutMsg) Hash() Hash {
	var h Hash
	hash := sha256.New()
	hash.Write(timeout.View.ToBytes())
	if qc, ok := timeout.SyncInfo.QC(); ok {
		h := qc.BlockHash()
		hash.Write(h[:])
	}
	hash.Write(timeout.ID.ToBytes())
	hash.Sum(h[:0])
	return h
}

func (timeout TimeoutMsg) String() string {
	return fmt.Sprintf("TimeoutMsg{ ID: %d, View: %d, SyncInfo: %v }", timeout.ID, timeout.View, timeout.SyncInfo)
}

// NewViewMsg is sent to the leader whenever a replica decides to advance to the next view.
// It contains the highest QC or TC known to the replica.
type NewViewMsg struct {
	ID       hotstuff.ID // The ID of the replica who sent the message.
	SyncInfo SyncInfo    // The highest QC / TC.
}

// CommitEvent is raised whenever a block is committed,
// and includes the number of client commands that were executed.
type CommitEvent struct {
	Commands int
}
