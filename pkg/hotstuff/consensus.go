package hotstuff

import (
	"context"
	"time"
)

func createLeaf(parent *Node, cmd []byte, qc *QuorumCert, height int) *Node {

	return &Node{
		ParentHash: parent.Hash(),
		Command:    cmd,
		Justify:    qc,
		Height:     height,
	}
}

func (r *Replica) Update(node *Node) {
	node1, _ := r.nodes.Get(node.Justify.hash)
	node2, _ := r.nodes.Get(node1.Justify.hash)
	node3, _ := r.nodes.Get(node2.Justify.hash)

	r.pm.UpdateQCHigh(*node.Justify)

	if node2.Height > r.bLock.Height {
		r.bLock = node2
	}

	parent1, _ := r.nodes.Get(node1.ParentHash)
	parent2, _ := r.nodes.Get(node2.ParentHash)

	if parent1 == node2 && parent2 == node3 {
		r.commit(node3)
		r.bExec = node3
	}
}

func (r *Replica) commit(node *Node) {

	if r.bLock.Height < node.Height {
		if parent, ok := r.nodes.Parent(node); ok {
			r.commit(parent)
			// exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
		}
	}

}

func receiveProposal() {} // this should be a quorum server function

func onReciveVote() {} // this should be a function the quorum

func propose(leafNode *Node, cmd []byte, highQC *QuorumCert, r *Replica) {

	newNode := createLeaf(leafNode, cmd, highQC, leafNode.Height+1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	r.config.Propose(ctx, newNode.toProto())
	cancel()
} // this is the quorum call the client(the leader) makes.

// The idea of the flow is that propose is called -> reciveProposal receives the propsal sendt by propose. recives sends the votes back to
// propose(poropse is blocking atm). After propose stops blocking, due to geting the needed info from receiveProposal, onReciveVote is triggerd by propose.
