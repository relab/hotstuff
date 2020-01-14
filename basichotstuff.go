package hotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/hotstuff/proto"
	"google.golang.org/grpc"
)

var replicas = []ReplicaInfo{
	ReplicaInfo{Address: "127.0.0.1:8080", ID: 1, Leader: true},
	ReplicaInfo{Address: "127.0.0.1:8081", ID: 2, Leader: false},
	ReplicaInfo{Address: "127.0.0.1:8082", ID: 3, Leader: false},
	ReplicaInfo{Address: "127.0.0.1:8083", ID: 4, Leader: false},
}

type ReplicaInfo struct {
	Address string
	ID      int
	Leader  bool
}

type Replica struct {
	mu       sync.Mutex
	leaderId int
	config   *proto.Configuration
	manager  *proto.Manager

	viewNumber int
	lockedQC   *proto.QuorumCert
}

func NewReplica(leaderId int) *Replica {
	return &Replica{
		leaderId:   leaderId,
		viewNumber: 1,
	}
}

//this is a leader, gives high moral yeees.
type Leader struct {
	*Replica

	Majority int
}

func NewLeader(id, majority int) *Leader {
	r := NewReplica(id)
	return &Leader{
		Replica:  r,
		Majority: majority,
	}
}

func mostCommonMatchingMsgs(msgs []*proto.Msg) *proto.Msg {

	length := len(msgs)

	matches := 0

	msgMap := make(map[*proto.Msg]int)

	for i := range msgs {
		for j := range msgs {
			if i < length-1-j {
				if comparingMsgs(msgs[i], msgs[length-1-j]) {
					matches++
				}
			} else {
				break
			}

		}
		if _, exist := msgMap[msgs[i]]; exist {
			msgMap[msgs[i]] += matches

		} else {
			msgMap[msgs[i]] = matches
		}
		matches = 0
	}

	var mostMatches *proto.Msg

	high := 0

	for k, v := range msgMap {
		if high < v {
			mostMatches = k
		}
	}

	return mostMatches
}

func sign(signers []*proto.Msg) string {
	signature := ""

	for i := range signers {
		signature += strconv.Itoa(i)
	}
	return signature
}

func (l *Leader) BroadcastQF(req *proto.Msg, replies []*proto.Msg) (*proto.QuorumCert, bool) {
	if len(replies) < l.Majority {
		return nil, false
	}

	matching := make([]*proto.Msg, 0, len(replies))
	for _, msg := range replies {
		if matchingMsg(msg, req.GetType(), req.GetViewNumber()) {
			matching = append(matching, msg)
		}
	}

	if len(matching) < l.Majority {
		return nil, false
	}

	// msg := mostCommonMatchingMsgs(replies)

	qc := &proto.QuorumCert{
		Type:       req.GetType(),
		ViewNumber: req.GetViewNumber(),
		Node:       req.GetNode(),
		Sig:        sign(matching),
	}

	return qc, true
}

// This starts the protocol.
func (l *Leader) RunHotStuff(reps []*Replica) {

	l.creatClientConnection()
	for i, r := range replicas {
		reps[i].startServer(r.Address)
	}

	for {

	}

}

// the leader acting as a client

func (r *Replica) creatClientConnection() bool {

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
	conf, err := mgr.NewConfiguration(ids, l)
	if err != nil {
		log.Fatalln("error creating read config:", err)
	}

	r.config = conf

	return true
}

func matchingMsg(m *proto.Msg, t proto.Type, v int32) bool {

	return m.Type == t && m.ViewNumber == v

}

func matchingQC(qc *proto.QuorumCert, t proto.Type, v int32) bool {

	return qc.Type == t && qc.ViewNumber == v
}

func comparingMsgs(m1 *proto.Msg, m2 *proto.Msg) bool {
	ok := 0
	if m1.Type == m2.Type && m1.ViewNumber == m2.ViewNumber {
		ok++
	}
	if m1.Node.Command == m2.Node.Command && m1.Node.ParentHash == m2.Node.ParentHash {
		ok++
	}

	if ok == 2 {
		return true
	}
	return false
}

// replicas actiong as servers

func (r *Replica) Broadcast(ctx context.Context, msg *proto.Msg) (msg *proto.Msg, err error) {

	return msg, nil
}

func (l *Leader) NewView(ctx context.Context, msg *proto.Msg) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (r *Replica) startServer(port string) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterHotstuffLeaderServer(grpcServer, r)
	grpcServer.Serve(lis)
}
