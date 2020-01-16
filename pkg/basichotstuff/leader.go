package basichotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/hotstuff/pkg/proto"
	"google.golang.org/grpc"
)

//Leader is responisble for proposing new commands to be executed.
type Leader struct {
	*Replica

	Majority int

	leaderManager      *proto.Manager
	leaderConfig       *proto.Configuration
	commands           <-chan string
	CurrentNewViewMsgs []*proto.Msg
	contextCancel      context.CancelFunc
}

//NewLeader creats a new leader object.
func NewLeader(replica *Replica, majority int, commands <-chan string) *Leader {
	return &Leader{
		Replica:  replica,
		Majority: majority,
		commands: commands,
	}
}

//Init sets up the required connections for communication.
func (l *Leader) Init(addresses []string, leaderPort string, timeout time.Duration) (err error) {
	listenPort := "42069" //ayyy lmafo
	leaderAddr := "127.0.0.1:42070"
	err = l.Replica.Init(listenPort, leaderAddr, timeout)
	if err != nil {
		return err
	}
	err = l.createClientConnection(addresses)
	if err != nil {
		return err
	}
	err = l.serveClients(leaderPort)
	return err
}

func (l *Leader) createClientConnection(reps []string) error {
	mgr, err := proto.NewManager(reps, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(500*time.Millisecond),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}

	l.leaderManager = mgr

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	conf, err := mgr.NewConfiguration(ids, l)
	if err != nil {
		return err
	}

	l.leaderConfig = conf

	return nil
}

func (l *Leader) serveClients(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterHotstuffReplicaServer(grpcServer, l)
	grpcServer.Serve(lis)
	return nil
}

//BroadcastQF creats and send the qc's.
func (l *Leader) BroadcastQF(req *proto.Msg, replies []*proto.Msg) (*proto.QuorumCert, bool) {
	if len(replies) < l.Majority {
		return nil, false
	}

	// replicas will return NEW_VIEW on DECIDE phase
	var t proto.Type
	if req.GetType() == proto.DECIDE {
		t = proto.NEW_VIEW
	} else {
		t = req.GetType()
	}

	matching := make([]*proto.Msg, 0, len(replies))
	for _, msg := range replies {
		if matchingMsg(msg, t, req.GetViewNumber()) {
			matching = append(matching, msg)
		}
	}

	if len(matching) < l.Majority {
		return nil, false
	}

	if t == proto.NEW_VIEW {
		l.CurrentNewViewMsgs = matching
		return nil, true
	}

	// msg := mostCommonMatchingMsgs(replies)

	signature := l.Crypto.Combine(req.GetType(), req.GetViewNumber(), req.GetNode(), matching)

	qc := &proto.QuorumCert{
		Type:       req.GetType(),
		ViewNumber: req.GetViewNumber(),
		Node:       req.GetNode(),
		Sig:        signature.String(),
	}

	return qc, true
}

//Broadcast is a wrapper for the brodcasts that are made.
func (l *Leader) Broadcast(msg *proto.Msg) (QC *proto.QuorumCert, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	l.contextCancel = cancel
	QC, err = l.leaderConfig.Broadcast(ctx, msg)
	cancel()
	return
}

//NewView handels new view messages.
func (l *Leader) NewView(ctx context.Context, msg *proto.Msg) (*empty.Empty, error) {
	if !matchingMsg(msg, msg.GetType(), msg.GetViewNumber()) {
		return &empty.Empty{}, nil
	}

	l.Replica.mu.Lock()
	defer l.Replica.mu.Unlock()
	l.CurrentNewViewMsgs = append(l.CurrentNewViewMsgs, msg)

	if l.Majority <= len(l.CurrentNewViewMsgs) {
		if l.contextCancel != nil {
			l.contextCancel()
		}
		go l.Prepare()
	}

	return &empty.Empty{}, nil
}

//Prepare executs the prepare phase.
func (l *Leader) Prepare() {
	var maxVN *proto.Msg

	for _, msg := range l.CurrentNewViewMsgs {
		if msg.GetViewNumber() > maxVN.GetViewNumber() && l.Crypto.Verify(msg.GetJustify()) {
			maxVN = msg
		}
	}

	l.CurrentNewViewMsgs = nil

	highQC := maxVN.GetJustify()

	var command string

	// non blocking read of command
	// TODO: figure out what to do if there is no command
	// maybe spin on nextview interrupt
	// maby have a pause state
	select {
	case command = <-l.commands:
	default:
		command = ""
	}

	// Need to check that parent exists
	parent, ok := l.Nodes[highQC.GetNode().GetParentHash()]
	if !ok {
		// but its just a prototype, so who cares
		log.Fatalf("Node contained parent which does not exist!")
	}

	node := &proto.HSNode{
		ParentHash: HashNode(parent),
		Command:    command,
	}

	msg := &proto.Msg{
		Type:       proto.PREPARE,
		ViewNumber: l.ViewNumber,
		Node:       node,
		Justify:    highQC,
	}

	// TODO: figure out how to broadcast to the leader aswell
	prepareQC, err := l.Broadcast(msg)
	if err != nil {
		log.Fatalf("Error on PREPARE: %v\n", err)
	}
	l.Precommit(prepareQC)
}

//Precommit executes the pre-commit phase.
func (l *Leader) Precommit(prepareQC *proto.QuorumCert) {
	msg := &proto.Msg{
		Type:       proto.PRE_COMMIT,
		ViewNumber: l.ViewNumber,
		Node:       nil,
		Justify:    prepareQC,
	}
	precommitQC, err := l.Broadcast(msg)
	if err != nil {
		log.Fatalf("Error on PRECOMMIT: %v\n", err)
	}
	l.Commit(precommitQC)
}

//Commit executes the commit phase.
func (l *Leader) Commit(precommitQC *proto.QuorumCert) {
	msg := &proto.Msg{
		Type:       proto.COMMIT,
		ViewNumber: l.ViewNumber,
		Node:       nil,
		Justify:    precommitQC,
	}
	commitQC, err := l.Broadcast(msg)
	if err != nil {
		log.Fatalf("Error on COMMIT: %v\n", err)
	}
	l.Decide(commitQC)
}

//Decide makes the decision to give a green ligth to a command.
func (l *Leader) Decide(commitQC *proto.QuorumCert) {
	msg := &proto.Msg{
		Type:       proto.DECIDE,
		ViewNumber: l.ViewNumber,
		Node:       nil,
		Justify:    commitQC,
	}
	_, err := l.Broadcast(msg)
	if err != nil {
		log.Fatalf("Error on DECIDE: %v\n", err)
	}
	l.Prepare()
}
