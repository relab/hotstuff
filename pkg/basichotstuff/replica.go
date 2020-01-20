package basichotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/relab/hotstuff/pkg/crypto"
	"github.com/relab/hotstuff/pkg/proto"

	"google.golang.org/grpc"
)

//Replica is the struct for replica machines. They are responsible for voting in nodes and QC's that carry commands that is to be executed.
type Replica struct {
	mu            sync.Mutex
	client        proto.HotstuffReplicaClient
	genesis       *proto.HSNode
	cancelTimeout context.CancelFunc

	ViewNumber int32
	lockedQC   *proto.QuorumCert
	prepareQC  *proto.QuorumCert
	Crypto     crypto.Crypto
	Nodes      map[string]*proto.HSNode // map of HashNode(.) to Node of the committed node
	Logger     *log.Logger
}

//NewReplica creats a new replica and returns a replica object.
func NewReplica() *Replica {
	genesis := &proto.HSNode{}
	nodes := make(map[string]*proto.HSNode)
	nodes[HashNode(genesis)] = genesis
	return &Replica{
		ViewNumber: 0,
		Crypto:     crypto.NoCrypto{},
		genesis:    genesis,
		Nodes:      nodes,
		prepareQC: &proto.QuorumCert{
			Type:       proto.PREPARE,
			ViewNumber: 0,
			Node:       genesis,
		},
		lockedQC: &proto.QuorumCert{
			Type:       proto.PRE_COMMIT,
			ViewNumber: -1,
		},
		Logger: log.New(os.Stderr, "hotstuff: ", log.Flags()),
	}
}

//Init sets up the replica listener and sets up a normal grpc connection from the replica to its leader.
func (r *Replica) Init(listenPort, leaderAddr string, timeout time.Duration) (err error) {
	err = r.serveLeader(listenPort)
	if err != nil {
		return err
	}
	err = r.dialLeader(leaderAddr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancelTimeout = cancel
	// run NewView with timeout
	go r.newViewTimeout(ctx, timeout)
	return nil
}

func (r *Replica) serveLeader(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterHotstuffLeaderServer(grpcServer, r)
	go grpcServer.Serve(lis)
	return nil
}

func (r *Replica) dialLeader(address string) error {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to dial leader: %w", err)
	}

	r.client = proto.NewHotstuffReplicaClient(conn)
	return nil
}

// Broadcast is the function that receives a message from the leader and responds with a vote
func (r *Replica) Broadcast(ctx context.Context, msg *proto.Msg) (*proto.Msg, error) {
	// TODO: check if it is safe to take the leader's viewnumber at this point (its not)
	if msg.GetViewNumber() > r.ViewNumber {
		r.ViewNumber = msg.GetViewNumber()
	}

	if matchingMsg(msg, proto.PREPARE, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PREPARE, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrecommit(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PRE_COMMIT, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandleCommit(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.COMMIT, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandleDecide(msg), nil
	}

	return r.Msg(proto.GENERIC, nil, nil), nil
}

//SafeNode checks if a node exstends , lockedQC's node and if highQC's view number is higher than lockedQC's. Returns true if one of the checks is ok.
func (r *Replica) SafeNode(node *proto.HSNode, qc *proto.QuorumCert) bool {
	extends := false
	if parent, ok := r.Nodes[HashNode(node)]; r.lockedQC.GetNode() != nil && ok &&
		HashNode(r.lockedQC.GetNode()) == HashNode(parent) {
		extends = true
	}

	return extends || r.lockedQC.GetViewNumber() < qc.GetViewNumber()
}

func (r *Replica) voteMsg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
	msg = r.Msg(typ, node, qc)
	msg.PartialSig = r.Crypto.Sign(msg).String()

	return
}

//Msg creats a proto.Msg object.
func (r *Replica) Msg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
	if node == nil {
		node = &proto.HSNode{}
	}
	if qc == nil {
		qc = &proto.QuorumCert{}
	}

	msg = &proto.Msg{
		Type:       typ,
		Node:       node,
		ViewNumber: r.ViewNumber,
		Justify:    qc,
	}

	return
}

//HandlePrepare handels brodcasts from the leader sendt in the prepare phase.
func (r *Replica) HandlePrepare(msg *proto.Msg) *proto.Msg {
	if parent, ok := r.Nodes[msg.Node.GetParentHash()]; ok && HashNode(parent) == HashNode(msg.Justify.GetNode()) &&
		r.SafeNode(msg.Node, msg.Justify) {
		r.Logger.Printf("PREPARE (%d)\n", r.ViewNumber)
		return r.voteMsg(proto.PREPARE, msg.GetNode(), nil)
	}
	return r.Msg(proto.GENERIC, nil, nil)
}

//HandlePrecommit handels brodcasts from the leader sendt in the pre-commit phase.
func (r *Replica) HandlePrecommit(msg *proto.Msg) *proto.Msg {
	r.prepareQC = msg.GetJustify()
	r.Logger.Printf("PRECOMMIT (%d)\n", r.ViewNumber)
	return r.voteMsg(proto.PRE_COMMIT, msg.GetJustify().GetNode(), nil)
}

//HandleCommit handels brodcasts from the leader sendt in the commit phase.
func (r *Replica) HandleCommit(msg *proto.Msg) *proto.Msg {
	r.lockedQC = msg.GetJustify()
	r.Logger.Printf("COMMIT (%d)\n", r.ViewNumber)
	return r.voteMsg(proto.COMMIT, msg.GetJustify().GetNode(), nil)
}

//HandleDecide handels brodcast from the leader sendt in the dicede phase. The command is executed and a new view message is sendt to the leader.
func (r *Replica) HandleDecide(msg *proto.Msg) *proto.Msg {
	// execute command
	// proceed to next view => inc ViewCount and send NEW_VIEW
	r.Logger.Printf("DECIDE (%d): %s", r.ViewNumber, r.lockedQC.GetNode().GetCommand())
	nw := r.Msg(proto.NEW_VIEW, nil, r.prepareQC)
	r.ViewNumber++
	return nw
}

//NewView sends a grpc packet to the leader, requesting that is new view starts. This is sendt if a replica times out.
func (r *Replica) NewView() {
	r.Logger.Printf("NEWVIEW (%d)\n", r.ViewNumber)
	msg := r.Msg(proto.NEW_VIEW, nil, r.prepareQC)
	r.ViewNumber++
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err := r.client.NewView(ctx, msg)
	if err != nil {
		r.Logger.Println("Failed to send NewView to leader: ", err)
		// dont start a new view if unable to contact leader
		r.ViewNumber--
	}
	cancel()
}

func (r *Replica) newViewTimeout(ctx context.Context, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		r.NewView()
	case <-ctx.Done():
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancelTimeout = cancel
	r.newViewTimeout(ctx, timeout)
}
