package hotstuff

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/relab/hotstuff/pkg/proto"
	"google.golang.org/grpc"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stderr, "hs: ", log.Lshortfile|log.Ltime|log.Lmicroseconds)
	if os.Getenv("HOTSTUFF_LOG") != "1" {
		logger.SetOutput(ioutil.Discard)
	}
}

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	proto.HotstuffClient
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

	server  *grpc.Server
	manager *proto.Manager
	config  *proto.Configuration
	qspec   *hotstuffQSpec

	Pacemaker
	// Notifications will be sent to pacemaker on this channel
	pmNotifyChan chan Notification

	// interaction with the outside application happens through these
	Commands <-chan []byte
	Exec     func([]byte)
}

// New creates a new Hotstuff instance
func New(id ReplicaID, privKey *ecdsa.PrivateKey, config *ReplicaConfig, pacemaker Pacemaker, timeout time.Duration,
	exec func([]byte)) *HotStuff {
	logger.SetPrefix(fmt.Sprintf("hs(id %d): ", id))

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
		Pacemaker:     pacemaker,
		nodes:         nodes,
		genesis:       genesis,
		bLock:         genesis,
		bExec:         genesis,
		bLeaf:         genesis,
		qcHigh:        qcForGenesis,
		timeout:       timeout,
		Exec:          exec,
		pmNotifyChan:  make(chan Notification, 10),
	}
}

// Init starts the networking components of hotstuff
func (hs *HotStuff) Init(port string) error {
	err := hs.startServer(port)
	if err != nil {
		return fmt.Errorf("Failed to start GRPC Server: %w", err)
	}
	err = hs.startClient()
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
		hs.pmNotify(Notification{HQCUpdate, newQCHighNode, qc})
		return true
	}

	logger.Println("UpdateQCHigh Failed")
	return false
}

// OnReceiveProposal handles a replica's response to the Proposal from the leader
func (hs *HotStuff) onReceiveProposal(node *Node) (*PartialCert, error) {
	logger.Println("OnReceiveProposal: ", node)
	hs.nodes.Put(node)
	defer hs.update(node)

	if node.Height > hs.vHeight && hs.safeNode(node) {
		logger.Println("OnReceiveProposal: Accepted node")
		hs.vHeight = node.Height
		pc, err := CreatePartialCert(hs.id, hs.privKey, node)
		hs.pmNotify(Notification{ReceiveProposal, node, nil})
		return pc, err
	}
	logger.Println("OnReceiveProposal: Node not accepted")
	return nil, fmt.Errorf("Node was not accepted")
}

// OnReceiveNewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *HotStuff) onReceiveNewView(qc *QuorumCert) {
	logger.Println("OnReceiveNewView")
	node, _ := hs.nodes.NodeOf(qc)
	hs.pmNotify(Notification{ReceiveNewView, node, qc})
}

// OnReceiveLeaderChange handles an incoming LeaderUpdate message for a new leader.
func (hs *HotStuff) onReceiveLeaderChange(qc *QuorumCert, sig partialSig) {
	logger.Println("OnReceiveLeaderChange: vHeight: ", hs.vHeight)
	hash := sha256.Sum256(qc.toBytes())
	//The hs.pm.GetLeader should return this hs here and not the old leader.
	//There is currently no way of knowing who the old leader were.
	info, ok := hs.Replicas[hs.GetLeader(hs.vHeight)]
	if ok && ecdsa.Verify(info.PubKey, hash[:], sig.r, sig.s) {
		hs.UpdateQCHigh(qc)
		node, _ := hs.nodes.Get(qc.hash)
		hs.pmNotify(Notification{QCFinish, node, qc})
		// TODO: start a new proposal
		// A new round of proposals might already have begun?
		// The QCFinish notification has already been sent to the pacemaker in a goroutine at this point.
		// Maybe consider locking this, or sending the QCFinish notification here.
	} else {
		logger.Println("OnReceiveLeaderChange: did not accept incoming qc")
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
	if !hs.UpdateQCHigh(node.Justify) && !bytes.Equal(node.Justify.toBytes(), hs.qcHigh.toBytes()) {
		// it is expected that updateQCHigh will return false if qcHigh equals qc.
		// this happens when the leader already got the qc after a proposal
		logger.Println("UpdateQCHigh failed, but qc did not equal qcHigh")
	}

	node2, ok := hs.nodes.NodeOf(node1.Justify)
	if !ok || node2.Committed {
		return
	}

	if node2.Height > hs.bLock.Height {
		hs.bLock = node2 // COMMIT on node2
		logger.Println("COMMIT: ", node2)
	}

	node3, ok := hs.nodes.NodeOf(node2.Justify)
	if !ok || node3.Committed {
		return
	}

	if node1.ParentHash == node2.Hash() && node2.ParentHash == node3.Hash() {
		hs.commit(node3)
		hs.bExec = node3 // DECIDE on node3
		logger.Println("DECIDE ", node3)
	}

}

func (hs *HotStuff) commit(node *Node) {
	if hs.bExec.Height < node.Height {
		if parent, ok := hs.nodes.ParentOf(node); ok {
			hs.commit(parent)
			node.Committed = true
			logger.Println("EXEC ", node)
			hs.Exec(node.Command) // execute the commmand in the node. this is the last step in a nodes life.
		}
	}
}

// Propose will fetch a command from the Commands channel and proposes it to the other replicas
func (hs *HotStuff) Propose(cmd []byte) {
	logger.Printf("Propose (cmd: %.15s)\n", cmd)
	newNode := createLeaf(hs.bLeaf, cmd, hs.qcHigh, hs.bLeaf.Height+1)
	hs.pmNotify(Notification{Propose, newNode, hs.qcHigh})

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

	// set the QSpec's QC to our newQC
	hs.qspec.QC = newQC

	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	_, err = hs.config.Propose(ctx, newNode.toProto())
	cancel()

	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
		// TODO: Figure out what to do here (maybe nothing?)
		// return
	}

	hs.bLeaf = newNode
	hs.UpdateQCHigh(hs.qspec.QC)

	hs.pmNotify(Notification{QCFinish, newNode, newQC})
} // this is the quorum call the client(the leader) makes.

// doLeaderChange will sign the qc and forward it to the leader of vHeight+1
func (hs *HotStuff) doLeaderChange(qc *QuorumCert) {
	hash := sha256.Sum256(qc.toBytes())
	R, S, err := ecdsa.Sign(rand.Reader, hs.privKey, hash[:])
	if err != nil {
		logger.Println("Failed to sign QC for leader change: ", err)
		return
	}
	leader := hs.GetLeader(hs.vHeight + 1)
	logger.Println("doLeaderChange: changing to leader: ", leader)
	sig := &partialSig{hs.id, R, S}
	upd := &proto.LeaderUpdate{QC: qc.toProto(), Sig: sig.toProto()}
	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	info, ok := hs.Replicas[leader]
	if !ok || info.ID != leader {
		panic(fmt.Errorf("Failed to get info for leader: %d", leader))
	}
	_, err = info.LeaderChange(ctx, upd)
	if err != nil {
		logger.Println("Failed to perform leader change: ", err)
	}
	cancel()
}

// SendNewView sends a newView message to the leader replica.
func (hs *HotStuff) SendNewView() error {
	logger.Println("SendNewView")
	ctx, cancel := context.WithTimeout(context.Background(), hs.timeout)
	defer cancel()
	replica := hs.Replicas[hs.GetLeader(hs.vHeight+1)]
	if replica.ID == hs.id {
		// "send" new-view to self
		hs.pmNotify(Notification{ReceiveNewView, hs.bLeaf, hs.qcHigh})
		return nil
	}
	_, err := replica.NewView(ctx, hs.qcHigh.toProto())
	if err != nil {
		logger.Println("Failed to send new view to leader: ", err)
		return err
	}
	return nil
}

func createLeaf(parent *Node, cmd []byte, qc *QuorumCert, height int) *Node {
	return &Node{
		ParentHash: parent.Hash(),
		Command:    cmd,
		Justify:    qc,
		Height:     height,
	}
}

func (hs *HotStuff) startClient() error {
	// sort addresses based on ID, excluding self
	ids := make([]ReplicaID, 0, len(hs.Replicas)-1)
	addrs := make([]string, 0, len(hs.Replicas)-1)
	for _, replica := range hs.Replicas {
		if replica.ID != hs.id {
			i := sort.Search(len(ids), func(i int) bool { return ids[i] >= replica.ID })
			ids = append(ids, 0)
			copy(ids[i+1:], ids[i:])
			ids[i] = replica.ID
			addrs = append(addrs, "")
			copy(addrs[i+1:], addrs[i:])
			addrs[i] = replica.Address
		}
	}

	mgr, err := proto.NewManager(addrs, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(hs.timeout),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}
	hs.manager = mgr

	nodes := mgr.Nodes()
	for i, id := range ids {
		hs.Replicas[id].HotstuffClient = nodes[i].HotstuffClient
	}

	hs.QuorumSize = len(hs.Replicas) - (len(hs.Replicas)-1)/3
	logger.Println("Majority: ", hs.QuorumSize)

	hs.qspec = &hotstuffQSpec{
		ReplicaConfig: hs.ReplicaConfig,
	}

	hs.config, err = hs.manager.NewConfiguration(hs.manager.NodeIDs(), hs.qspec)
	if err != nil {
		return fmt.Errorf("Failed to create configuration: %w", err)
	}

	return nil
}

// startServer runs a new instance of hotstuffServer
func (hs *HotStuff) startServer(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	hs.server = grpc.NewServer()
	hsServer := &hotstuffServer{hs}
	proto.RegisterHotstuffServer(hs.server, hsServer)
	go hs.server.Serve(lis)
	return nil
}

// Close closes all connections made by the HotStuff instance
func (hs *HotStuff) Close() {
	hs.manager.Close()
	hs.server.Stop()
	close(hs.pmNotifyChan)
}
