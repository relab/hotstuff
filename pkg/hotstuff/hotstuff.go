package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
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

type ReplicaID int

type ReplicaInfo struct {
	ID     ReplicaID
	Socket string
	PubKey *ecdsa.PublicKey
}

type ReplicaConfig struct {
	Replicas   map[ReplicaID]ReplicaInfo
	QuorumSize int
}

type Replica struct {
	*ReplicaConfig

	mu      sync.Mutex
	vHeight int
	bLock   *Node
	bExec   *Node

	id ReplicaID

	nodes NodeStorage
	pm    Pacemaker

	manager *proto.Manager
	config  *proto.Configuration

	privKey *ecdsa.PrivateKey
	pubKey  *ecdsa.PublicKey
}

func (r *Replica) createClientConnection(addressInfo []string, timeout time.Duration) error {
	mgr, err := proto.NewManager(addressInfo, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(timeout),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}

	r.manager = mgr

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	conf, err := mgr.NewConfiguration(ids, r)
	if err != nil {
		return err
	}

	r.config = conf

	return nil
}

// this is in practicalety the onPropose function

func (r *Replica) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.QuorumCert, bool) {
	if len(replies) < r.ReplicaConfig.QuorumSize {
		return nil, false
	}
	qc := CreateQuorumCert(nodeFromProto(req))
	for _, pc := range replies {
		qc.AddPartial(partialCertFromProto(pc))
	}
	r.pm.UpdateQCHigh(*qc)
	protoQC := qc.toProto()

	return protoQC, true
}

func (r *Replica) serveBrodcast(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterHotstuffServer(grpcServer, r)
	go grpcServer.Serve(lis)
	return nil
}

func (r *Replica) Propose(ctx context.Context, node *proto.HSNode) (*proto.PartialCert, error) {
	normalNode := nodeFromProto(node)

	defer r.Update(normalNode) // update is in the consensus file

	if normalNode.Height > r.vHeight && safeNode(*r, *normalNode) {
		r.vHeight = normalNode.Height
		pc, _ := CreatePartialCert(r.id, r.privKey, normalNode)
		return pc.toProto(), nil
	}

	return nil, nil
}

func (r *Replica) NewView(ctx context.Context, msg *proto.QuorumCert) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func safeNode(r Replica, node Node) bool {
	parent, _ := r.nodes.Get(node.ParentHash)
	qcNode, _ := r.nodes.Node(node.Justify)
	return parent == r.bLock || qcNode.Height > r.bLock.Height
}
