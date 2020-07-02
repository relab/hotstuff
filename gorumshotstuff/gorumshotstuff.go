package gorumshotstuff

import (
	"context"
	"fmt"
	"log"
	"net"
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
	qspec   *HotstuffQSpec

	closeOnce sync.Once

	qcTimeout      time.Duration
	connectTimeout time.Duration
}

//New creates a new GorumsHotStuff backend object.
func New(connectTimeout, qcTimeout time.Duration) *GorumsHotStuff {
	return &GorumsHotStuff{
		replicaInfo:    make(map[hotstuff.ReplicaID]*gorumsReplica),
		connectTimeout: connectTimeout,
		qspec: &HotstuffQSpec{
			SignatureCache: nil,
			ReplicaConfig:  nil,
			bestReplicaID:  0,
		},
		qcTimeout: qcTimeout,
	}
}

//GetQSpec returns the qspec used by the backend.
func (hs *GorumsHotStuff) GetQSpec() *HotstuffQSpec {
	return hs.qspec
}

//DoPropose is the interface between backend and consensus logic when sending a propoasl.
func (hs *GorumsHotStuff) DoPropose(block *hotstuff.Block) (*hotstuff.QuorumCert, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hs.qcTimeout)
	defer cancel()
	pb := proto.BlockToProto(block)
	hs.qspec.Reset()
	pbAndID := &proto.BlockAndLeaderID{
		Block:    pb,
		LeaderID: uint32(hs.HotStuff.GetID()),
	}
	pqc, err := hs.config.Propose(ctx, pbAndID)
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
	QCandID := &proto.QCAndID{
		QC: pb,
		Id: uint32(hs.GetID()),
	}
	_, err := info.node.NewView(ctx, QCandID)
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
	/*	addrs := make([]string, 0, len(hs.Replicas)-1)
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
	*/
	addrs := make(map[string]uint32)
	for _, replica := range hs.Replicas { //Looping over a map here. The address map is not being generated in a determinisktic manner. Wired stuff sometimes happens.
		if replica.ID != hs.GetID() {
			ids = append(ids, replica.ID)
			addrs[replica.Address] = uint32(replica.ID)
			logger.Println(replica.ID)
		}
	}

	temp := make([]hotstuff.ReplicaID, len(ids))
	index := 0
	for _, node1 := range ids {
		for _, node2 := range ids {
			if node2 < node1 {
				index++
			}
		}
		temp[index] = node1
		index = 0
	}
	ids = temp

	mgr, err := proto.NewManager(proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(connectTimeout),
		proto.WithSpesifiedNodeID(addrs),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}
	hs.manager = mgr
	// Det er en bug her med at id og node.id ikke er den samme. Dette skyldes sansynligvis av at inputen til manageren ikke er sortert.
	nodes := mgr.Nodes()
	logger.Println(nodes)
	for i, id := range ids {
		hs.replicaInfo[id].node = nodes[i]
		logger.Println("-----")
		logger.Println(id)
		logger.Println(nodes[i].ID())
		logger.Println("======")

	}

	logger.Println(addrs)
	hs.qspec.SignatureCache = hs.SigCache
	hs.qspec.ReplicaConfig = hs.ReplicaConfig

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
func (hs *GorumsHotStuff) Propose(blockAndLeaderID *proto.BlockAndLeaderID) *proto.PartialCert {
	p, err := hs.OnReceiveProposal(blockAndLeaderID.Block.FromProto(), blockAndLeaderID.LeaderID)
	if err != nil {
		logger.Println("OnReceiveProposal returned with error: ", err)
		return &proto.PartialCert{}
	}
	return proto.PartialCertToProto(p)
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *GorumsHotStuff) NewView(msg *proto.QCAndID) *proto.Empty {
	qc := msg.QC.FromProto()
	hs.OnReceiveNewView(qc, hotstuff.ReplicaID(msg.Id))
	return &proto.Empty{}
}
