package basichotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/relab/hotstuff/pkg/crypto"
	"github.com/relab/hotstuff/pkg/proto"

	"google.golang.org/grpc"
)

type ReplicaInfo struct {
	Address string
	ID      int
	Leader  bool
}

type Replica struct {
	mu       sync.Mutex
	leaderId int
	client   proto.HotstuffReplicaClient
	/* config   *proto.Configuration
	manager  *proto.Manager */

	ViewNumber int32
	lockedQC   *proto.QuorumCert
	prepareQC  *proto.QuorumCert
	replicas   []ReplicaInfo
	Crypto     crypto.Crypto
	Nodes      map[string]*proto.HSNode // map of HashNode(.) to Node of the committed node
}

func NewReplica(leaderId int, replicas []ReplicaInfo) *Replica {
	return &Replica{
		leaderId:   leaderId,
		ViewNumber: 1,
		replicas:   replicas,
		Crypto:     crypto.NoCrypto{},
	}
}

func (r *Replica) serveLeader(port string) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v\n", err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterHotstuffLeaderServer(grpcServer, r)
	grpcServer.Serve(lis)
}

func (r *Replica) dialLeader(address string) {
	conn, err := grpc.Dial(address)
	if err != nil {
		log.Fatalf("failed to dial leader: %v\n", err)
	}
	defer conn.Close()
	r.client = proto.NewHotstuffReplicaClient(conn)
}

/* func (r *Replica) creatClientConnection() bool {

	var addrs []string
	addrs = append(addrs, "127.0.0.1:8080")

	mgr, err := proto.NewManager(addrs, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(500*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}

	r.manager = mgr

	// Get all all available node ids, 3 nodes
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	conf, err := mgr.NewConfiguration(ids)
	if err != nil {
		log.Fatalln("error creating read config:", err)
	}

	r.config = conf

	return true
} */
// Broadcast is the function that receives a message from the leader and responds with a vote
func (r *Replica) Broadcast(ctx context.Context, msg *proto.Msg) (*proto.Msg, error) {
	if matchingMsg(msg, proto.PREPARE, r.ViewNumber) {
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PREPARE, r.ViewNumber) {
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.PRE_COMMIT, r.ViewNumber) {
		return r.HandlePrepare(msg), nil
	}

	if matchingQC(msg.GetJustify(), proto.COMMIT, r.ViewNumber) {
		return r.HandlePrepare(msg), nil
	}

	return nil, nil
}

func (r *Replica) SafeNode(node *proto.HSNode, qc *proto.QuorumCert) bool {
	return r.Nodes[node.GetParentHash()] == r.lockedQC.Node || r.lockedQC.GetViewNumber() < qc.GetViewNumber()
}

func (r *Replica) voteMsg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
	msg = &proto.Msg{
		Type:    typ,
		Node:    node,
		Justify: qc,
	}

	msg.PartialSig = r.Crypto.Sign(msg).String()
	return
}

func Msg(typ proto.Type, node *proto.HSNode, qc *proto.QuorumCert) (msg *proto.Msg) {
	msg = &proto.Msg{
		Type:    typ,
		Node:    node,
		Justify: qc,
	}

	return
}

func (r *Replica) HandlePrepare(msg *proto.Msg) *proto.Msg {
	if parent, ok := r.Nodes[msg.Node.GetParentHash()]; ok && HashNode(parent) == HashNode(msg.Justify.GetNode()) &&
		r.SafeNode(msg.Node, msg.Justify) {
		return r.voteMsg(proto.PREPARE, msg.GetNode(), nil)
	}
	return nil
}

func (r *Replica) HandlePrecommit(msg *proto.Msg) *proto.Msg {
	r.prepareQC = msg.GetJustify()
	return r.voteMsg(proto.PRE_COMMIT, msg.GetJustify().GetNode(), nil)
}

func (r *Replica) HandleCommit(msg *proto.Msg) *proto.Msg {
	r.lockedQC = msg.GetJustify()
	return r.voteMsg(proto.COMMIT, msg.GetJustify().GetNode(), nil)
}

func (r *Replica) HandleDecide(msg *proto.Msg) *proto.Msg {
	// execute command
	// proceed to next view => inc ViewCount and send NEW_VIEW
	return Msg(proto.NEW_VIEW, nil, r.prepareQC)
}

func (r *Replica) NewView() {
	msg := Msg(proto.NEW_VIEW, nil, r.prepareQC)
	r.ViewNumber += 1
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err := r.client.NewView(ctx, msg)
	if err != nil {
		log.Fatalf("Failed to send new view: %v\n", err)
	}
	cancel()
}
