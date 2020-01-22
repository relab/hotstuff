package hotstuff

import (
	"github.com/relab/hotstuff/pkg/proto"
)

func createLeaf(parent storage.Node, cmd string, qc QuorumCert, height int) storage.Node {

	newNode := storage.Node{
		parent:  parent,
		justify: qc,
		height:  height,
	}

	return newNode
}

func (r *Replica) update(node *Node) {
	node1 := node.justify.node
	node2 := node1.justify.node
	node3 := node2.justify.node

	pacemaker.updateQCHigh(node.justify)

	if node2.height > r.bLock.height {
		r.bLock = node2
	}

	if node1.parent == node2 && node2.parent == node3 {
		r.commit(node3)
		r.bExec = node3
	}
}

func (r *Replica) commit(node *storage.Node) {

	if r.bLock.height < node.height {
		r.commit(node.parent)
		exec(node.cmd) // execute the commmand in the node. this is the last step in a nodes life.
	}

}

func receiveProposal() {} // this should be a quorum server function

func onReciveVote() {} // this should be a function the quorum

func propose(leafNode Node, cmd string, highQC *proto.QuorumCert) {

	newNode := createLeaf(leafNode, cmd, highQC, leafNode.height+1)

	msg := *proto.Msg{
		Type:   *proto.GENERIC,
		HSNode: leafNode,
	}

	BordcastQF(msg)

	return newNode

} // this is the quorum call the client(the leader) makes.

// The idea of the flow is that propose is called -> reciveProposal receives the propsal sendt by propose. recives sends the votes back to
// propose(poropse is blocking atm). After propose stops blocking, due to geting the needed info from receiveProposal, onReciveVote is triggerd by propose.
