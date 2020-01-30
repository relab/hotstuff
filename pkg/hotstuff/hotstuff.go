package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/relab/hotstuff/pkg/proto"
	"google.golang.org/grpc"
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
	Replicas   map[ReplicaID]*ReplicaInfo
	QuorumSize int
}

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
	mut sync.Mutex

	// protocol data
	genesis *Node
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

	pm Pacemaker
	// Notifications will be sent to pacemaker on this channel
	pmNotifyChan chan Notification

	// interaction with the outside application happens through these
	Commands <-chan []byte
	Exec     func([]byte)
}

func New(id ReplicaID, privKey *ecdsa.PrivateKey, config *ReplicaConfig, pacemaker Pacemaker, timeout time.Duration,
	commands <-chan []byte, exec func([]byte)) *HotStuff {
	genesis := &Node{
		Committed: true,
	}
	qcForGenesis := CreateQuorumCert(genesis)
	nodes := NewMapStorage()
	nodes.Put(genesis)
	return &HotStuff{
		ReplicaConfig: config,
		id:            id,
		privKey:       privKey,
		pm:            pacemaker,
		nodes:         nodes,
		genesis:       genesis,
		bLock:         genesis,
		bExec:         genesis,
		bLeaf:         genesis,
		qcHigh:        qcForGenesis,
		timeout:       timeout,
		Commands:      commands,
		Exec:          exec,
		pmNotifyChan:  make(chan Notification),
	}
}

func (hs *HotStuff) Init(port string) error {
	logger.SetPrefix(fmt.Sprintf("hotstuff(id %d):", hs.id))

	err := hs.StartServer(port)
	if err != nil {
		return fmt.Errorf("Failed to start GRPC Server: %w", err)
	}
	err = hs.StartClient()
	if err != nil {
		return fmt.Errorf("Failed to start GRPC Clients: %w", err)
	}
	return nil
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
	logger.Println("UpdateQCHigh")
	if !VerifyQuorumCert(hs.ReplicaConfig, qc) {
		logger.Println("QC not verified!: ", qc)
		return false
	}

	newQCHighNode, ok := hs.nodes.NodeOf(qc)
	if !ok {
		logger.Println("Could not find node of new QC!")
		return false
	}

	oldQCHighNode, ok := hs.nodes.NodeOf(hs.qcHigh)
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
		logger.Println("OnReceiveProposal: Accepted node")
		hs.nodes.Put(node)
		hs.vHeight = node.Height
		pc, err := CreatePartialCert(hs.id, hs.privKey, node)
		go hs.pmNotify(Notification{ReceiveProposal, node, nil})
		return pc, err
	}
	logger.Println("OnReceiveProposal: Node not accepted")
	return nil, fmt.Errorf("Node was not accepted")
}

// OnReceiveNewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *HotStuff) onReceiveNewView(qc *QuorumCert) {
	logger.Println("OnReceiveNewView")
	hs.UpdateQCHigh(qc)
}

// OnReceiveLeaderChange handles an incoming LeaderUpdate message for a new leader.
func (hs *HotStuff) onReceiveLeaderChange(qc *QuorumCert, sig partialSig) {
	logger.Println("OnReceiveLeaderchange")
	hash := sha256.Sum256(qc.toBytes())
	info, ok := hs.Replicas[hs.pm.GetLeader()]
	if ok && ecdsa.Verify(info.PubKey, hash[:], sig.r, sig.s) {
		hs.UpdateQCHigh(qc)
		// TODO: start a new proposal
		// A new round of proposals might already have begun? The QCFinish notification has alredy been sendt to the pacemaker in a go rutine at this point.
		// Maby consider locking this, or sending the QCFinish notificarion here.
	}
}

func (hs *HotStuff) safeNode(node *Node) bool {
	qcNode, nExists := hs.nodes.NodeOf(node.Justify)
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
		}
		if !accept {
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

	hs.UpdateQCHigh(node.Justify) // PRE-COMMIT on node1

	node2, ok := hs.nodes.NodeOf(node1.Justify)
	if !ok || node2.Committed {
		return
	}

	if node2.Height > hs.bLock.Height {
		hs.bLock = node2 // COMMIT on node2
	}

	node3, ok := hs.nodes.NodeOf(node2.Justify)
	if !ok || node3.Committed {
		return
	}

	if node1.ParentHash == node2.Hash() && node2.ParentHash == node3.Hash() {
		hs.commit(node3)
		hs.bExec = node3 // DECIDE on node3
	}

}

func (hs *HotStuff) commit(node *Node) {
	if hs.bLock.Height < node.Height {
		if parent, ok := hs.nodes.ParentOf(node); ok {
			hs.commit(parent)
			node.Committed = true
			hs.Exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
		}
	}
}

// Propose will fetch a command from the Commands channel and proposes it to the other replicas
func (hs *HotStuff) Propose() {
	cmd := <-hs.Commands
	logger.Printf("Propose (cmd: %s)\n", cmd)
	newNode := createLeaf(hs.bLeaf, cmd, hs.qcHigh, hs.bLeaf.Height+1)
	go hs.pmNotify(Notification{Propose, newNode, hs.qcHigh})

	newQC := CreateQuorumCert(newNode)
	// self vote
	partialCert, err := hs.onReceiveProposal(newNode)
	if err != nil {
		panic(fmt.Errorf("Failed to vote for own proposal: %w", err))
	}
	err = newQC.AddPartial(partialCert)
	if err != nil {
		logger.Println("Failed to add own vote to QC: ", err)
	}

	qSpec := &hotstuffQSpec{
		ReplicaConfig: hs.ReplicaConfig,
		QC:            newQC,
	}
	config, err := hs.manager.NewConfiguration(hs.manager.NodeIDs(), qSpec)
	if err != nil {
		panic(fmt.Errorf("Failed to create configuration for propose QC"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	_, err = config.Propose(ctx, newNode.toProto())
	cancel()

	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
		return
	}

	hs.bLeaf = newNode

	if hs.pm.GetLeader() == hs.id {
		hs.UpdateQCHigh(qSpec.QC)
	} else {
		hs.doLeaderChange(qSpec.QC)
	}

	go hs.pmNotify(Notification{QCFinish, newNode, newQC})
} // this is the quorum call the client(the leader) makes.

// doLeaderChange will sign the qc and forward it to the leader
func (hs *HotStuff) doLeaderChange(qc *QuorumCert) {
	logger.Println("doLeaderChange")
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
	logger.Println("SendNewView")
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

func (hs *HotStuff) StartClient() error {
	addrs := make([]string, 0, len(hs.Replicas))
	for _, replica := range hs.Replicas {
		if replica.ID != hs.id {
			addrs = append(addrs, replica.Socket)
		}
	}

	mgr, err := proto.NewManager(addrs, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(hs.timeout),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}
	hs.manager = mgr

	nodes := mgr.Nodes()
	i := 0
	for _, replica := range hs.Replicas {
		if replica.ID != hs.id {
			replica.HotstuffClient = nodes[i].HotstuffClient
			i++
		}
	}

	hs.QuorumSize = len(hs.Replicas) - (len(hs.Replicas)-1)/3
	logger.Println("Majority: ", hs.QuorumSize)
	return nil
}

// StartServer runs a new instance of hotstuffServer
func (hs *HotStuff) StartServer(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	server := &hotstuffServer{hs}
	proto.RegisterHotstuffServer(grpcServer, server)
	go grpcServer.Serve(lis)
	return nil
}
