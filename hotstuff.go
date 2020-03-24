package hotstuff

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/waitutil"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	ID      ReplicaID
	Address string
	PubKey  *ecdsa.PublicKey
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	Replicas   map[ReplicaID]*ReplicaInfo
	QuorumSize int
}

// NewConfig returns a new ReplicaConfig instance
func NewConfig() *ReplicaConfig {
	return &ReplicaConfig{
		Replicas: make(map[ReplicaID]*ReplicaInfo),
	}
}

// Event is the type of notification sent to pacemaker
type Event uint8

// These are the types of events that can be sent to pacemaker
const (
	QCFinish Event = iota
	Propose
	ReceiveProposal
	HQCUpdate
	ReceiveNewView
)

// Notification sends information about a protocol state change to the pacemaker
type Notification struct {
	Event Event
	Node  *Node
	QC    *QuorumCert
}

// Backend is the networking code that serves HotStuff
type Backend interface {
	Init(*HotStuff)
	DoPropose(*Node, *QuorumCert) (*QuorumCert, error)
	DoNewView(ReplicaID, *QuorumCert) error
	Close()
}

// HotStuff is the safety core of the HotStuff protocol
type HotStuff struct {
	*ReplicaConfig
	mut sync.Mutex

	// depend on a network abstraction
	Backend

	// protocol data
	genesis *Node
	vHeight int
	bLock   *Node
	bExec   *Node

	// fields that pacemaker needs
	bLeaf  *Node
	qcHigh *QuorumCert

	id      ReplicaID
	nodes   NodeStorage
	privKey *ecdsa.PrivateKey

	// the duration that hotstuff can wait for an out-of-order message
	waitDuration time.Duration
	waitProposal *waitutil.WaitUtil

	// Notifications will be sent to pacemaker on this channel
	pmNotifyChan chan Notification

	// interaction with the outside application happens through these
	Commands <-chan []byte
	Exec     func([]byte)
}

// GetID returns the ID of this hotstuff instance
func (hs *HotStuff) GetID() ReplicaID {
	return hs.id
}

// GetHeight returns the height of the tree
func (hs *HotStuff) GetHeight() int {
	return hs.bLeaf.Height
}

// GetVotedHeight returns the height that was last voted at
func (hs *HotStuff) GetVotedHeight() int {
	return hs.vHeight
}

// GetLeafNode returns the current leaf node of the tree
func (hs *HotStuff) GetLeafNode() *Node {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	return hs.bLeaf
}

// SetLeafNode sets the leaf node of the tree
func (hs *HotStuff) SetLeafNode(node *Node) {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	hs.bLeaf = node
}

// GetQCHigh returns the highest valid Quorum Certificate known to the hotstuff instance.
func (hs *HotStuff) GetQCHigh() *QuorumCert {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	return hs.qcHigh
}

// New creates a new Hotstuff instance
func New(id ReplicaID, privKey *ecdsa.PrivateKey, config *ReplicaConfig, backend Backend,
	waitDuration time.Duration, exec func([]byte)) *HotStuff {
	logger.SetPrefix(fmt.Sprintf("hs(id %d): ", id))
	genesis := &Node{
		Committed: true,
	}
	qcForGenesis := CreateQuorumCert(genesis)
	nodes := NewMapStorage()
	nodes.Put(genesis)
	hs := &HotStuff{
		ReplicaConfig: config,
		id:            id,
		Backend:       backend,
		privKey:       privKey,
		nodes:         nodes,
		genesis:       genesis,
		bLock:         genesis,
		bExec:         genesis,
		bLeaf:         genesis,
		qcHigh:        qcForGenesis,
		waitDuration:  waitDuration,
		waitProposal:  waitutil.NewWaitUtil(),
		Exec:          exec,
		pmNotifyChan:  make(chan Notification, 10),
	}

	backend.Init(hs)
	return hs
}

// GetNotifier returns a channel where the pacemaker can listen for notifications
func (hs *HotStuff) GetNotifier() <-chan Notification {
	return hs.pmNotifyChan
}

func (hs *HotStuff) pmNotify(n Notification) {
	hs.pmNotifyChan <- n
}

// This function will expect to return the node for the qc if it is delivered within a duration.
func (hs *HotStuff) expectNodeFor(qc *QuorumCert) (node *Node, ok bool) {
	hs.waitProposal.WaitCondTimeout(hs.waitDuration, func() bool {
		node, ok = hs.nodes.NodeOf(qc)
		return ok
	})
	return
}

// UpdateQCHigh updates the qc held by the paceMaker, to the newest qc.
func (hs *HotStuff) UpdateQCHigh(qc *QuorumCert) bool {
	logger.Println("UpdateQCHigh")
	if !VerifyQuorumCert(hs.ReplicaConfig, qc) {
		logger.Println("QC not verified!: ", qc)
		return false
	}

	newQCHighNode, ok := hs.expectNodeFor(qc)
	if !ok {
		logger.Println("Could not find node of new QC!")
		return false
	}

	hs.mut.Lock()
	defer hs.mut.Unlock()

	oldQCHighNode, ok := hs.nodes.NodeOf(hs.qcHigh)
	if !ok {
		panic(fmt.Errorf("Node from the old qcHigh missing from storage"))
	}

	if newQCHighNode.Height > oldQCHighNode.Height {
		hs.qcHigh = qc
		hs.bLeaf = newQCHighNode
		hs.pmNotify(Notification{HQCUpdate, newQCHighNode, qc})
		return true
	}

	logger.Println("UpdateQCHigh Failed")
	return false
}

// OnReceiveProposal handles a replica's response to the Proposal from the leader
func (hs *HotStuff) OnReceiveProposal(node *Node) (*PartialCert, error) {
	logger.Println("OnReceiveProposal: ", node)
	hs.nodes.Put(node)

	// wake anyone waiting for a proposal
	hs.waitProposal.WakeAll()

	defer hs.update(node)

	hs.mut.Lock()
	defer hs.mut.Unlock()

	if node.Height > hs.vHeight && hs.safeNode(node) {
		logger.Println("OnReceiveProposal: Accepted node")
		hs.vHeight = node.Height
		pc, err := CreatePartialCert(hs.id, hs.privKey, node)
		hs.pmNotify(Notification{ReceiveProposal, node, hs.qcHigh})
		return pc, err
	}
	logger.Println("OnReceiveProposal: Node not accepted")
	return nil, fmt.Errorf("Node was not accepted")
}

// OnReceiveNewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *HotStuff) OnReceiveNewView(qc *QuorumCert) {
	logger.Println("OnReceiveNewView")
	hs.UpdateQCHigh(qc)
	node, _ := hs.nodes.NodeOf(qc)
	hs.pmNotify(Notification{ReceiveNewView, node, qc})
}

func (hs *HotStuff) safeNode(node *Node) bool {
	// safeNode is only called from onReceiveProposal, and is covered by its mutex lock.
	qcNode, nExists := hs.expectNodeFor(node.Justify)
	accept := false
	if nExists && qcNode.Height > hs.bLock.Height {
		accept = true
	} else {
		logger.Println("safeNode: liveness condition failed")
		// check if node extends bLock
		b := node
		ok := true
		for ok && b.Height > hs.bLock.Height+1 {
			b, ok = hs.nodes.Get(b.ParentHash)
		}
		if ok && b.ParentHash == hs.bLock.Hash() {
			accept = true
		} else {
			logger.Println("safeNode: safety condition failed")
		}
	}
	return accept
}

func (hs *HotStuff) update(node *Node) {
	// node1 = b'', node2 = b', node3 = b
	node1, ok := hs.nodes.NodeOf(node.Justify)
	if !ok || node1.Committed {
		return
	}

	logger.Println("PRE COMMIT: ", node1)
	// PRE-COMMIT on node1
	hs.UpdateQCHigh(node.Justify)

	/* if !hs.UpdateQCHigh(node.Justify) && !bytes.Equal(node.Justify.toBytes(), hs.QCHigh.toBytes()) {
		// it is expected that updateQCHigh will return false if qcHigh equals qc.
		// this happens when the leader already got the qc after a proposal
		logger.Println("UpdateQCHigh failed, but qc did not equal qcHigh")
	} */

	node2, ok := hs.nodes.NodeOf(node1.Justify)
	if !ok || node2.Committed {
		return
	}

	hs.mut.Lock()
	defer hs.mut.Unlock()

	if node2.Height > hs.bLock.Height {
		hs.bLock = node2 // COMMIT on node2
		logger.Println("COMMIT: ", node2)
	}

	node3, ok := hs.nodes.NodeOf(node2.Justify)
	if !ok || node3.Committed {
		return
	}

	if node1.ParentHash == node2.Hash() && node2.ParentHash == node3.Hash() {
		logger.Println("DECIDE ", node3)
		hs.commit(node3)
		hs.bExec = node3 // DECIDE on node3
	}
}

func (hs *HotStuff) commit(node *Node) {
	// only called from within update. Thus covered by its mutex lock.
	if hs.bExec.Height < node.Height {
		if parent, ok := hs.nodes.ParentOf(node); ok {
			hs.commit(parent)
		}
		node.Committed = true
		logger.Println("EXEC ", node)
		hs.Exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
	}
}

// Propose will fetch a command from the Commands channel and proposes it to the other replicas
func (hs *HotStuff) Propose(cmd []byte) {
	logger.Printf("Propose (cmd: %.15s)\n", cmd)
	hs.mut.Lock()

	newNode := CreateLeaf(hs.bLeaf, cmd, hs.qcHigh, hs.bLeaf.Height+1)
	hs.pmNotify(Notification{Propose, newNode, hs.qcHigh})

	newQC := CreateQuorumCert(newNode)

	hs.mut.Unlock()

	// self vote
	partialCert, err := hs.OnReceiveProposal(newNode)
	if err != nil {
		panic(fmt.Errorf("Failed to vote for own proposal: %w", err))
	}
	err = newQC.AddPartial(partialCert)
	if err != nil {
		logger.Println("Failed to add own vote to QC: ", err)
	}

	qc, err := hs.DoPropose(newNode, newQC)

	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
		return
	}

	hs.mut.Lock()
	hs.bLeaf = newNode
	hs.mut.Unlock()

	hs.UpdateQCHigh(qc)

	hs.pmNotify(Notification{QCFinish, newNode, newQC})
} // this is the quorum call the client(the leader) makes.

// SendNewView sends a newView message to the leader replica.
func (hs *HotStuff) SendNewView(leader ReplicaID) error {
	logger.Println("SendNewView")
	replica := hs.Replicas[leader]
	if replica == nil {
		panic(fmt.Errorf("Could not find replica with id '%d'", leader))
	}
	if replica.ID == hs.id {
		// "send" new-view to self
		hs.pmNotify(Notification{ReceiveNewView, hs.bLeaf, hs.qcHigh})
		return nil
	}
	err := hs.DoNewView(leader, hs.GetQCHigh())
	if err != nil {
		logger.Println("Failed to send new view to leader: ", err)
		return err
	}
	return nil
}

// CreateLeaf returns a new node that extends the parent.
func CreateLeaf(parent *Node, cmd []byte, qc *QuorumCert, height int) *Node {
	return &Node{
		ParentHash: parent.Hash(),
		Command:    cmd,
		Justify:    qc,
		Height:     height,
	}
}
