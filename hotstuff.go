package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/data"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader(view int) config.ReplicaID
	Init(*HotStuff)
}

// HotStuff is a thing
type HotStuff struct {
	*consensus.HotStuffCore
	tls bool

	pacemaker Pacemaker

	nodes map[config.ReplicaID]*proto.Node

	server  *hotstuffServer
	manager *proto.Manager
	cfg     *proto.Configuration

	closeOnce sync.Once

	qcTimeout      time.Duration
	connectTimeout time.Duration

	sugestedLeader config.ReplicaID
}

// SetSugestedLeader sets the sugested leader id to the id given as input.
func (hs *HotStuff) SetSugestedLeader(id config.ReplicaID) {
	hs.sugestedLeader = id
}

//New creates a new GorumsHotStuff backend object.
func New(conf *config.ReplicaConfig, pacemaker Pacemaker, tls bool, connectTimeout, qcTimeout time.Duration) *HotStuff {
	hs := &HotStuff{
		pacemaker:      pacemaker,
		HotStuffCore:   consensus.New(conf),
		nodes:          make(map[config.ReplicaID]*proto.Node),
		connectTimeout: connectTimeout,
		qcTimeout:      qcTimeout,
		sugestedLeader: 0,
	}
	pacemaker.Init(hs)
	return hs
}

//Start starts the server and client
func (hs *HotStuff) Start() error {
	addr := hs.Config.Replicas[hs.Config.ID].Address
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

func (hs *HotStuff) startClient(connectTimeout time.Duration) error {
	idMapping := make(map[string]uint32, len(hs.Config.Replicas)-1)
	for _, replica := range hs.Config.Replicas {
		if replica.ID != hs.Config.ID {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", hs.Config.ID),
	})

	perNodeMD := func(nid uint32) metadata.MD {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], nid)
		hash := sha256.Sum256(b[:])
		R, S, err := ecdsa.Sign(rand.Reader, hs.Config.PrivateKey, hash[:])
		if err != nil {
			panic(fmt.Errorf("Could not sign proof for replica %d: %w", nid, err))
		}
		md := metadata.MD{}
		md.Append("proof", base64.StdEncoding.EncodeToString(R.Bytes()), base64.StdEncoding.EncodeToString(S.Bytes()))
		return md
	}

	mgrOpts := []proto.ManagerOption{
		proto.WithDialTimeout(connectTimeout),
		proto.WithNodeMap(idMapping),
		proto.WithMetadata(md),
		proto.WithPerNodeMetadata(perNodeMD),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	}

	if hs.tls {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(hs.Config.CertPool, "")))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgrOpts = append(mgrOpts, proto.WithGrpcDialOptions(grpcOpts...))

	mgr, err := proto.NewManager(mgrOpts...)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}
	hs.manager = mgr

	for _, node := range mgr.Nodes() {
		hs.nodes[config.ReplicaID(node.ID())] = node
	}

	hs.cfg, err = hs.manager.NewConfiguration(hs.manager.NodeIDs(), &struct{}{})
	if err != nil {
		return fmt.Errorf("Failed to create configuration: %w", err)
	}

	return nil
}

// startServer runs a new instance of hotstuffServer
func (hs *HotStuff) startServer(port string) error {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}

	serverOpts := []proto.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if hs.tls {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(credentials.NewServerTLSFromCert(hs.Config.Cert)))
	}

	serverOpts = append(serverOpts, proto.WithGRPCServerOptions(grpcServerOpts...))

	hs.server = newHotStuffServer(hs, proto.NewGorumsServer(serverOpts...))
	hs.server.RegisterHotstuffServer(hs.server)

	go hs.server.Serve(lis)
	return nil
}

// Close closes all connections made by the HotStuff instance
func (hs *HotStuff) Close() {
	hs.closeOnce.Do(func() {
		hs.HotStuffCore.Close()
		hs.manager.Close()
		hs.server.Stop()
	})
}

// Propose broadcasts a new proposal to all replicas
func (hs *HotStuff) Propose() {
	proposal := hs.CreateProposal()
	logger.Printf("Propose (%d commands): %s\n", len(proposal.Commands), proposal)
	protobuf := &proto.BlockAndNewLeader{Block: proto.BlockToProto(proposal), SugestedNewLeaderID: uint32(hs.sugestedLeader)}
	hs.cfg.Propose(protobuf)
	// self-vote
	hs.handlePropose(proposal)
}

// SendNewView sends a NEW-VIEW message to a specific replica
func (hs *HotStuff) SendNewView(id config.ReplicaID) {
	qc := hs.GetQCHigh()
	if node, ok := hs.nodes[id]; ok {
		node.NewView(proto.QuorumCertToProto(qc))
	}
}

func (hs *HotStuff) handlePropose(block *data.Block) {
	p, err := hs.OnReceiveProposal(block)
	if err != nil {
		logger.Println("OnReceiveProposal returned with error:", err)
		return
	}
	leaderID := hs.pacemaker.GetLeader(block.Height)
	if hs.Config.ID == leaderID {
		hs.OnReceiveVote(p)
	} else if leader, ok := hs.nodes[leaderID]; ok {
		leader.Vote(proto.PartialCertToProto(p))
	}
}

type hotstuffServer struct {
	*HotStuff
	*proto.GorumsServer
	// maps a stream context to client info
	mut     sync.RWMutex
	clients map[context.Context]config.ReplicaID
}

func newHotStuffServer(hs *HotStuff, srv *proto.GorumsServer) *hotstuffServer {
	hsSrv := &hotstuffServer{
		HotStuff:     hs,
		GorumsServer: srv,
		clients:      make(map[context.Context]config.ReplicaID),
	}
	return hsSrv
}

func (hs *hotstuffServer) getClientID(ctx context.Context) (config.ReplicaID, error) {
	hs.mut.RLock()
	// fast path for known stream
	if id, ok := hs.clients[ctx]; ok {
		hs.mut.RUnlock()
		return id, nil
	}

	hs.mut.RUnlock()
	hs.mut.Lock()
	defer hs.mut.Unlock()

	// cleanup finished streams
	for ctx := range hs.clients {
		if ctx.Err() != nil {
			delete(hs.clients, ctx)
		}
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, fmt.Errorf("getClientID: metadata not available")
	}

	v := md.Get("id")
	if len(v) < 1 {
		return 0, fmt.Errorf("getClientID: id field not present")
	}

	id, err := strconv.Atoi(v[0])
	if err != nil {
		return 0, fmt.Errorf("getClientID: cannot parse ID field: %w", err)
	}

	info, ok := hs.Config.Replicas[config.ReplicaID(id)]
	if !ok {
		return 0, fmt.Errorf("getClientID: could not find info about id '%d'", id)
	}

	v = md.Get("proof")
	if len(v) < 2 {
		return 0, fmt.Errorf("getClientID: No proof found")
	}

	var R, S big.Int
	v0, err := base64.StdEncoding.DecodeString(v[0])
	if err != nil {
		return 0, fmt.Errorf("getClientID: could not decode proof: %v", err)
	}
	v1, err := base64.StdEncoding.DecodeString(v[1])
	if err != nil {
		return 0, fmt.Errorf("getClientID: could not decode proof: %v", err)
	}
	R.SetBytes(v0)
	S.SetBytes(v1)

	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(hs.Config.ID))
	hash := sha256.Sum256(b[:])

	if !ecdsa.Verify(info.PubKey, hash[:], &R, &S) {
		return 0, fmt.Errorf("Invalid proof")
	}

	hs.clients[ctx] = config.ReplicaID(id)
	return config.ReplicaID(id), nil
}

// Propose handles a replica's response to the Propose QC from the leader
func (hs *hotstuffServer) Propose(ctx context.Context, protoB *proto.BlockAndNewLeader) {
	block := protoB.Block.FromProto()
	id, err := hs.getClientID(ctx)
	if err != nil {
		logger.Printf("Failed to get client ID: %v", err)
		return
	}
	// defaults to 0 if error
	block.Proposer = id
	// TODO: replace the following with handlePropose
	p, err := hs.OnReceiveProposal(block)
	if err != nil {
		logger.Println("OnReceiveProposal returned with error:", err)
		return
	}
	leaderID := hs.pacemaker.GetLeader(block.Height)
	if p != nil && protoB.SugestedNewLeaderID != 0 {
		leaderID = config.ReplicaID(protoB.SugestedNewLeaderID)
	}
	if hs.Config.ID == leaderID {
		hs.OnReceiveVote(p)
	} else if leader, ok := hs.nodes[leaderID]; ok {
		leader.Vote(proto.PartialCertToProto(p))
	}
}

func (hs *hotstuffServer) Vote(ctx context.Context, cert *proto.PartialCert) {
	hs.OnReceiveVote(cert.FromProto())
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *hotstuffServer) NewView(ctx context.Context, msg *proto.QuorumCert) {
	qc := msg.FromProto()
	hs.OnReceiveNewView(qc)
}
