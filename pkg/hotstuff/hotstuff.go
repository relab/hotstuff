package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/relab/hotstuff/pkg/proto"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stderr, "hotstuff: ", log.Flags())
	if os.Getenv("HOTSTUFF_LOG") != "1" {
		logger.SetOutput(ioutil.Discard)
	}
}

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	proto.HotstuffClient
	ID     ReplicaID
	Socket string
	PubKey *ecdsa.PublicKey
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	Replicas   map[ReplicaID]ReplicaInfo
	QuorumSize int
}

// Event is the type of notification sent to pacemaker
type Event uint8

// These are the types of events that can be sent to pacemaker
const (
	QCFinish Event = iota
	Propose
	ReceiveProposal
	HQCUpdate
)

// Notification sends information about a protocol state change to the pacemaker
type Notification struct {
	Event Event
	Node  *Node
	QC    *QuorumCert
}

// HotStuff is an instance of the hotstuff protocol
type HotStuff struct {
	*ReplicaConfig
	mu sync.Mutex

	// protocol data
	vHeight int
	bLock   *Node
	bExec   *Node
	bLeaf   *Node
	qcHigh  *QuorumCert

	id      ReplicaID
	nodes   NodeStorage
	privKey *ecdsa.PrivateKey
	timeout time.Duration

	manager *proto.Manager
	config  *proto.Configuration

	pm Pacemaker
	// Notifications will be sent to pacemaker on this channel
	pmNotifyChan chan Notification

	// interaction with the outside application happens through these
	Commands <-chan []byte
	Exec     func([]byte)
}

// GetNotifier returns a channel where the pacemaker can listen for notifications
func (hs *HotStuff) GetNotifier() <-chan Notification {
	return hs.pmNotifyChan
}

func (hs *HotStuff) pmNotify(n Notification) {
	hs.pmNotifyChan <- n
}

// UpdateQCHigh updates the qc held by the paceMaker, to the newest qc.
func (hs *HotStuff) UpdateQCHigh(qc *QuorumCert) bool {
	if !VerifyQuorumCert(hs.ReplicaConfig, qc) {
		return false
	}

	newQCHighNode, ok := hs.nodes.Get(qc.hash)
	if !ok {
		return false
	}

	oldQCHighNode, ok := hs.nodes.Get(hs.qcHigh.hash)
	if !ok {
		panic(fmt.Errorf("Node from the old qcHigh missing from storage"))
	}

	if newQCHighNode.Height > oldQCHighNode.Height {
		hs.qcHigh = qc
		hs.bLeaf = newQCHighNode
		go hs.pmNotify(Notification{HQCUpdate, newQCHighNode, qc})
		return true
	}
	return false
}

// OnReceiveProposal handles a replica's response to the Proposal from the leader
func (hs *HotStuff) onReceiveProposal(node *Node) (*PartialCert, error) {
	defer hs.update(node)

	if node.Height > hs.vHeight && hs.safeNode(node) {
		hs.vHeight = node.Height
		pc, err := CreatePartialCert(hs.id, hs.privKey, node)
		go hs.pmNotify(Notification{HQCUpdate, node, nil})
		return pc, err
	}

	return nil, nil
}

// OnReceiveNewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *HotStuff) onReceiveNewView(qc *QuorumCert) {
	hs.UpdateQCHigh(qc)
}

// OnReceiveLeaderChange handles an incoming LeaderUpdate message for a new leader.
func (hs *HotStuff) onReceiveLeaderChange(qc *QuorumCert, sig partialSig) {
	hash := sha256.Sum256(qc.toBytes())
	info, ok := hs.Replicas[hs.pm.GetLeader()]
	if ok && ecdsa.Verify(info.PubKey, hash[:], sig.r, sig.s) {
		hs.UpdateQCHigh(qc)
		// TODO: start a new proposal
	}
}

func (hs *HotStuff) safeNode(node *Node) bool {
	parent, _ := hs.nodes.Get(node.ParentHash)
	qcNode, _ := hs.nodes.Node(node.Justify)
	return parent == hs.bLock || qcNode.Height > hs.bLock.Height
}

func (hs *HotStuff) update(node *Node) {
	// node1 = b'', node2 = b', node3 = b
	node1, _ := hs.nodes.Get(node.Justify.hash)
	node2, _ := hs.nodes.Get(node1.Justify.hash)
	node3, _ := hs.nodes.Get(node2.Justify.hash)

	hs.UpdateQCHigh(node.Justify)

	if node2.Height > hs.bLock.Height {
		hs.bLock = node2
	}

	parent1, _ := hs.nodes.Get(node1.ParentHash)
	parent2, _ := hs.nodes.Get(node2.ParentHash)

	if parent1 == node2 && parent2 == node3 {
		hs.commit(node3)
		hs.bExec = node3
	}
}

func (hs *HotStuff) commit(node *Node) {

	if hs.bLock.Height < node.Height {
		if parent, ok := hs.nodes.Parent(node); ok {
			hs.commit(parent)
			// TODO: Implement some way to execute the command
			hs.Exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
		}
	}

}

// Propose will fetch a command from the Commands channel and proposes it to the other replicas
func (hs *HotStuff) Propose() *Node {
	cmd := <-hs.Commands
	newNode := createLeaf(hs.bLeaf, cmd, hs.qcHigh, hs.bLeaf.Height+1)

	go hs.pmNotify(Notification{Propose, newNode, hs.qcHigh})
	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	qc, err := hs.config.Propose(ctx, newNode.toProto())

	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
	} else {
		newQC := quorumCertFromProto(qc)
		go hs.pmNotify(Notification{QCFinish, newNode, newQC})
		if hs.pm.GetLeader() != hs.id {
			hs.doLeaderChange(newQC)
		}
	}

	cancel()
	return newNode
} // this is the quorum call the client(the leader) makes.

// doLeaderChange will sign the qc and forward it to the leader
func (hs *HotStuff) doLeaderChange(qc *QuorumCert) {
	hash := sha256.Sum256(qc.toBytes())
	R, S, err := ecdsa.Sign(rand.Reader, hs.privKey, hash[:])
	if err != nil {
		logger.Println("Failed to sign QC for leader change: ", err)
		return
	}
	sig := &partialSig{hs.id, R, S}
	upd := &proto.LeaderUpdate{QC: qc.toProto(), Sig: sig.toProto()}
	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	_, err = hs.Replicas[hs.pm.GetLeader()].LeaderChange(ctx, upd)
	if err != nil {
		logger.Println("Failed to perform leader update: ", err)
	}
	cancel()
}

// SendNewView sends a newView message to the leader replica.
func (hs *HotStuff) SendNewView() {
	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	_, err := hs.Replicas[hs.pm.GetLeader()].NewView(ctx, hs.qcHigh.toProto())
	if err != nil {
		logger.Println("Failed to send new view to leader: ", err)
	}
	cancel()
}

func createLeaf(parent *Node, cmd []byte, qc *QuorumCert, height int) *Node {
	return &Node{
		ParentHash: parent.Hash(),
		Command:    cmd,
		Justify:    qc,
		Height:     height,
	}
}
