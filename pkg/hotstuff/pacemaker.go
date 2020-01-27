package hotstuff

import "context"

import "time"

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader() ReplicaID
	Beat(cmd []byte)
	NextSyncView()
	UpdateQCHigh(newQC *QuorumCert)
}

//StandardPacemaker is the standard implementation of the pacmaker interface.
type StandardPacemaker struct {
	qcHigh    *QuorumCert
	bLeaf     *Node
	replica   *Replica
	GetLeader func() ReplicaID
}

//UpdateQCHigh updates the qc held by the paceMaker, to the newest qc.
func (p *StandardPacemaker) UpdateQCHigh(qc *QuorumCert) bool {

	newQCHighNode, _ := p.replica.nodes.Get(qc.hash)

	oldQCHighNode, _ := p.replica.nodes.Get(p.qcHigh.hash)

	if newQCHighNode.Height > oldQCHighNode.Height {

		p.qcHigh = qc
		p.bLeaf, _ = p.replica.nodes.Get(p.qcHigh.hash)

		return true
	}
	return false
}

//Beat make the leader brodcast a new proposal for a node to work on.
func (p *StandardPacemaker) Beat(cmd []byte) {

	if p.replica.id == p.GetLeader() {
		p.bLeaf = propose(p.bLeaf, cmd, p.qcHigh, p.replica)
	}

}

//NextSyncView sends a newView message to the leader replica.
func (p *StandardPacemaker) NextSyncView() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err := p.replica.Replicas[p.GetLeader()].HotstuffClient.NewView(ctx, p.qcHigh.toProto())
	if err != nil {
		logger.Println("Failed to send new view to leader: ", err)
	}
	cancel()
}
