package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"github.com/relab/hotstuff/pkg/proto"
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

func (r *Replica) update(node *Node) {
	node1, _ := r.nodes.Get(node.Justify.hash)
	node2, _ := r.nodes.Get(node1.Justify.hash)
	node3, _ := r.nodes.Get(node2.Justify.hash)

	r.pm.UpdateQCHigh(node.Justify)

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
			// TODO: Implement some way to execute the command
			// exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
		}
	}

}

func receiveProposal() {} // this should be a quorum server function

func onReciveVote() {} // this should be a function the quorum

func propose(leafNode *Node, cmd []byte, highQC *QuorumCert, r *Replica) *Node {

	newNode := createLeaf(leafNode, cmd, highQC, leafNode.Height+1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	qc, err := r.config.Propose(ctx, newNode.toProto())
	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
	} else {
		r.doLeaderChange(quorumCertFromProto(qc))
	}
	cancel()
	return newNode
} // this is the quorum call the client(the leader) makes.

func (r *Replica) doLeaderChange(qc *QuorumCert) {
	hash := sha256.Sum256(qc.toBytes())
	R, S, err := ecdsa.Sign(rand.Reader, r.privKey, hash[:])
	if err != nil {
		logger.Println("Failed to sign QC for leader change: ", err)
		return
	}
	sig := &partialSig{r.id, R, S}
	upd := &proto.LeaderUpdate{QC: qc.toProto(), Sig: sig.toProto()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err = r.Replicas[r.pm.GetLeader()].HotstuffClient.LeaderChange(ctx, upd)
	if err != nil {
		logger.Println("Failed to perform leader update: ", err)
	}
	cancel()
}

// The idea of the flow is that propose is called -> reciveProposal receives the propsal sendt by propose. recives sends the votes back to
// propose(poropse is blocking atm). After propose stops blocking, due to geting the needed info from receiveProposal, onReciveVote is triggerd by propose.
