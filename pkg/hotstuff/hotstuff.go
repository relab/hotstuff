package hotstuff

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
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

type ReplicaID int

type ReplicaInfo struct {
	ID     ReplicaID
	Socket string
	PubKey *ecdsa.PublicKey
}

type ReplicaConfig struct {
	Replicas map[ReplicaID]ReplicaInfo
	Majority int
}

type Replica struct {
	mu      sync.Mutex
	vHeight int
	bLock   *Node
	bExec   *Node

	id   ReplicaID
	conf ReplicaConfig

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

func (r *Replica) BroadcastQF(req *proto.Msg, replies []*proto.Msg) (*proto.QuorumCert, bool) {

	

	return nil, true
}
