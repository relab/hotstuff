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
	childOfGenisis := &proto.HSNode{
		ParentHash: HashNode(genesis),
		Command:    "",
	}
	nodes[HashNode(childOfGenisis)] = childOfGenisis
	return &Replica{
		ViewNumber: 0,
		Crypto:     crypto.NoCrypto{},
		genesis:    genesis,
		Nodes:      nodes,
		prepareQC: &proto.QuorumCert{
			Type:       proto.PREPARE,
			ViewNumber: 0,
			Node:       childOfGenisis,
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
	// send the first new view
	r.NewView()
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
	if matchingMsg(msg, proto.PREPARE, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PREPARE, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PRE_COMMIT, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.COMMIT, r.ViewNumber) {
		r.cancelTimeout()
		return r.HandlePrepare(msg), nil
	}

	return nil, nil
}

//SafeNode checks if a node exstends , lockedQC's node and if highQC's view number is higher than lockedQC's. Returns true if one of the checks is ok.
func (r *Replica) SafeNode(node *proto.HSNode, qc *proto.QuorumCert) bool {
	return r.Nodes[node.GetParentHash()] == r.lockedQC.Node || r.lockedQC.GetViewNumber() < qc.GetViewNumber()
}

func (r *Replica) voteMsg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
	msg = &proto.Msg{
		Type:       typ,
		Node:       node,
		ViewNumber: r.ViewNumber,
		Justify:    qc,
	}

	msg.PartialSig = r.Crypto.Sign(msg).String()
	return
}

//Msg creats a proto.Msg object.
func (r *Replica) Msg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
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
		// TODO: check if it is safe to take the leader's viewnumber at this point
		if msg.GetViewNumber() > r.ViewNumber {
			r.ViewNumber = msg.GetViewNumber()
		}
		r.Logger.Println("PREPARE")
		return r.voteMsg(proto.PREPARE, msg.GetNode(), nil)
	}
	return nil
}

//HandlePrecommit handels brodcasts from the leader sendt in the pre-commit phase.
func (r *Replica) HandlePrecommit(msg *proto.Msg) *proto.Msg {
	r.prepareQC = msg.GetJustify()
	r.Logger.Println("PRECOMMIT")
	return r.voteMsg(proto.PRE_COMMIT, msg.GetJustify().GetNode(), nil)
}

//HandleCommit handels brodcasts from the leader sendt in the commit phase.
func (r *Replica) HandleCommit(msg *proto.Msg) *proto.Msg {
	r.lockedQC = msg.GetJustify()
	r.Logger.Println("COMMIT")
	return r.voteMsg(proto.COMMIT, msg.GetJustify().GetNode(), nil)
}

//HandleDecide handels brodcast from the leader sendt in the dicede phase. The command is executed and a new view message is sendt to the leader.
func (r *Replica) HandleDecide(msg *proto.Msg) *proto.Msg {
	// execute command
	// proceed to next view => inc ViewCount and send NEW_VIEW
	fmt.Print(r.lockedQC.GetNode().GetCommand())
	r.Logger.Println("DECIDE")
	return r.Msg(proto.NEW_VIEW, nil, r.prepareQC)
}

//NewView sends a grpc packet to the leader, requesting that is new view starts. This is sendt if a replica times out.
func (r *Replica) NewView() {
	r.Logger.Println("NEWVIEW")
	msg := r.Msg(proto.NEW_VIEW, nil, r.prepareQC)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err := r.client.NewView(ctx, msg)
	if err != nil {
		r.Logger.Println("Failed to send NewView to leader: ", err)
	} else {
		// dont start a new view if unable to contact leader
		r.ViewNumber++
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
