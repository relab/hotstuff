package gorumshotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
	"github.com/relab/hotstuff/internal/logging"
	"google.golang.org/grpc"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

type gorumsReplica struct {
	*hotstuff.ReplicaInfo
	node *proto.Node
}

// GorumsHotStuff is a backend for HotStuff that uses Gorums
type GorumsHotStuff struct {
	*hotstuff.HotStuff

	replicaInfo map[hotstuff.ReplicaID]*gorumsReplica

	server  *proto.GorumsServer
	manager *proto.Manager
	config  *proto.Configuration
	qspec   *hotstuffQSpec

	closeOnce sync.Once

	qcTimeout      time.Duration
	connectTimeout time.Duration
}

//New creates a new GorumsHotStuff backend object.
func New(connectTimeout, qcTimeout time.Duration) *GorumsHotStuff {
	return &GorumsHotStuff{
		replicaInfo:    make(map[hotstuff.ReplicaID]*gorumsReplica),
		connectTimeout: connectTimeout,
		qcTimeout:      qcTimeout,
	}
}

//DoPropose is the interface between backend and consensus logic when sending a propoasl.
func (hs *GorumsHotStuff) DoPropose(block *hotstuff.Block) (*hotstuff.QuorumCert, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hs.qcTimeout)
	defer cancel()
	pb := proto.BlockToProto(block)
	hs.qspec.Reset()
	pqc, err := hs.config.Propose(ctx, pb)
	return pqc.FromProto(), err
}

//DoNewView is the interface between backend and consensus lgoic  when sending a new view message.
func (hs *GorumsHotStuff) DoNewView(id hotstuff.ReplicaID, qc *hotstuff.QuorumCert) error {
	ctx, cancel := context.WithTimeout(context.Background(), hs.qcTimeout)
	defer cancel()
	info, ok := hs.replicaInfo[id]
	if !ok {
		return fmt.Errorf("Replica with id '%d' not found", id)
	}
	pb := proto.QuorumCertToProto(qc)
	_, err := info.node.NewView(ctx, pb)
	return err
}

// Init sets up the backend with info from hotstuff core
func (hs *GorumsHotStuff) Init(hsc *hotstuff.HotStuff) {
	hs.HotStuff = hsc
	for id, info := range hsc.Replicas {
		hs.replicaInfo[id] = &gorumsReplica{
			ReplicaInfo: info,
		}
	}
}

//Start starts the server and client
func (hs *GorumsHotStuff) Start() error {
	addr := hs.replicaInfo[hs.GetID()].Address
	err := hs.startServer(addr)
	if err != nil {
		return fmt.Errorf("Failed to start GRPC Server: %w", err)
	}
	err = hs.startClient(hs.connectTimeout)
	if err != nil {
		return fmt.Errorf("Failed to start GRPC Clients: %w", err)
	}
	return nil
}

func (hs *GorumsHotStuff) startClient(connectTimeout time.Duration) error {
	// sort addresses based on ID, excluding self
	ids := make([]hotstuff.ReplicaID, 0, len(hs.Replicas)-1)
	addrs := make([]string, 0, len(hs.Replicas)-1)
	for _, replica := range hs.Replicas {
		if replica.ID != hs.GetID() {
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
		proto.WithDialTimeout(connectTimeout),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}
	hs.manager = mgr

	nodes := mgr.Nodes()
	for i, id := range ids {
		hs.replicaInfo[id].node = nodes[i]
	}

	hs.qspec = &hotstuffQSpec{
		SignatureCache: hs.SigCache,
		ReplicaConfig:  hs.ReplicaConfig,
	}

	hs.config, err = hs.manager.NewConfiguration(hs.manager.NodeIDs(), hs.qspec)
	if err != nil {
		return fmt.Errorf("Failed to create configuration: %w", err)
	}

	return nil
}

// startServer runs a new instance of hotstuffServer
func (hs *GorumsHotStuff) startServer(port string) error {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}

	hs.server = proto.NewGorumsServer()
	hs.server.RegisterProposeHandler(hs)
	hs.server.RegisterNewViewHandler(hs)

	go hs.server.Serve(lis)
	return nil
}

// Close closes all connections made by the HotStuff instance
func (hs *GorumsHotStuff) Close() {
	hs.closeOnce.Do(func() {
		hs.manager.Close()
		hs.server.Stop()
	})
}

// Propose handles a replica's response to the Propose QC from the leader
func (hs *GorumsHotStuff) Propose(block *proto.Block) *proto.PartialCert {
	p, err := hs.OnReceiveProposal(block.FromProto())
	if err != nil {
		logger.Println("OnReceiveProposal returned with error: ", err)
		return &proto.PartialCert{}
	}
	return proto.PartialCertToProto(p)
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *GorumsHotStuff) NewView(msg *proto.QuorumCert) *proto.Empty {
	qc := msg.FromProto()
	hs.OnReceiveNewView(qc)
	return &proto.Empty{}
}
