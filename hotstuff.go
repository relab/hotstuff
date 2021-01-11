package hotstuff

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/data"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// ID uniquely identifies a replica
type ID uint32

// View uniquely identifies a view
type View uint64

// Hash is a SHA256 hash
type Hash [32]byte

// Command is a client request to be executed by the consensus protocol
type Command string

type ToBytes interface {
	ToBytes() []byte
}

// PublicKey is the public part of a replica's key pair
type PublicKey interface{}

// Credentials are used to authenticate communication between replicas
type Credentials interface{}

// PrivateKey is the private part of a replica's key pair
type PrivateKey interface {
	PublicKey() PublicKey
}

// Signature is a signature of a block
type Signature interface {
	ToBytes
	// Signer returns the ID of the replica that generated the signature.
	Signer() ID
}

// PartialCert is a certificate for a block created by a single replica
type PartialCert interface {
	ToBytes
	// Signature returns the signature
	Signature() Signature
	// BlockHash returns the hash of the block that was signed
	BlockHash() *Hash
}

// QuorumCert is a certificate for a Block created by a quorum of replicas
type QuorumCert interface {
	ToBytes
	// BlockHash returns the hash of the block for which the certificate was created
	BlockHash() *Hash
}

// Signer implements the methods requried to create signatures and certificates
type Signer interface {
	// Sign signs a single block and returns the signature
	Sign(block Block) (cert PartialCert, err error)
	// CreateQuourmCert creates a from a list of partial certificates
	CreateQuorumCert(block Block, signatures []PartialCert) (cert QuorumCert, err error)
}

// Verifier implements the methods required to verify partial and quorum certificates
type Verifier interface {
	// VerifyPartialCert verifies a single partial certificate
	VerifyPartialCert(cert PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate
	VerifyQuorumCert(qc QuorumCert) bool
}

// Replica implements the methods that communicate with another replica
type Replica interface {
	// ID returns the replica's id
	ID() ID
	// address returns the replica's address
	Address() string
	// PublicKey returns the replica's public key
	PublicKey() PublicKey
	// Credentials returns the transport credentials of the replica
	Credentials() Credentials
	// Propose sends the block to the other replica
	Propose(block Block)
	// Vote sends the partial certificate to the other replica
	Vote(cert PartialCert)
	// NewView sends the quorum certificate to the other replica
	NewView(qc QuorumCert)
}

// Block is a proposal that is made during the consensus protocol
type Block interface {
	ToBytes
	// Hash returns the hash of the block
	Hash() *Hash
	// Proposer returns the id of the proposer
	Proposer() ID
	// Parent returns the hash of the parent block
	Parent() *Hash
	// Command returns the command
	Command() Command
	// Certificate returns the certificate that this block references
	Certificate() QuorumCert
	// View returns the view in which the block was proposed
	View() View
}

// BlockChain is a datastructure that stores a chain of blocks
type BlockChain interface {
	// Store stores a block in the blockchain
	Store(Block)
	// Get retrieves a block given its hash
	Get(*Hash) (Block, bool)
}

// Consensus implements a consensus protocol
type Consensus interface {
	// Config returns the configuration of this replica
	Config() Config
	// View returns the current view
	View() View
	// HighQC returns the highest QC known to the replica
	HighQC() QuorumCert
	// Leaf returns the last proposed block
	Leaf() Block
	// Propose proposes the given command
	Propose(cmd Command)
	// OnPropose handles an incoming proposal
	OnPropose(block Block)
	// OnVote handles an incoming vote
	OnVote(cert PartialCert)
	// OnVote handles an incoming NewView
	OnNewView(qc QuorumCert)
}

type Config interface {
	// ID returns the id of this replica
	ID() ID
	// PrivateKey returns the id of this replica
	PrivateKey() PrivateKey
	// Replicas returns all of the replicas in the configuration
	Replicas() map[ID]Replica
	// QuorumSize returns the size of a quorum
	QuorumSize() int
}

// LeaderRotation implements a leader rotation scheme
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view
	GetLeader(View) ID
}

// ViewSynchronizer TODO
type ViewSynchronizer interface {
}

var logger *zap.SugaredLogger

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

	pacemaker Pacemaker

	nodes map[config.ReplicaID]*proto.Node

	server  *hotstuffServer
	manager *proto.Manager
	cfg     *proto.Configuration

	closeOnce sync.Once

	connectTimeout time.Duration

	proposeCancel context.CancelFunc
	voteCancel    context.CancelFunc
	newviewCancel context.CancelFunc
}

//New creates a new GorumsHotStuff backend object.
func New(conf *config.ReplicaConfig, pacemaker Pacemaker, connectTimeout time.Duration) *HotStuff {
	hs := &HotStuff{
		pacemaker:      pacemaker,
		HotStuffCore:   consensus.New(conf),
		nodes:          make(map[config.ReplicaID]*proto.Node),
		connectTimeout: connectTimeout,
		proposeCancel:  func() {},
		voteCancel:     func() {},
		newviewCancel:  func() {},
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

	mgrOpts := []gorums.ManagerOption{
		gorums.WithDialTimeout(connectTimeout),
		gorums.WithNodeMap(idMapping),
		gorums.WithMetadata(md),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	}

	if hs.Config.Creds != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(hs.Config.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

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

	serverOpts := []gorums.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if hs.Config.Creds != nil {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(hs.Config.Creds.Clone()))
	}

	serverOpts = append(serverOpts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	hs.server = newHotStuffServer(hs, gorums.NewServer(serverOpts...))
	proto.RegisterHotstuffServer(hs.server.Server, hs.server)

	go hs.server.Serve(lis)
	return nil
}

// Close closes all connections made by the HotStuff instance
func (hs *HotStuff) Close() {
	hs.closeOnce.Do(func() {
		hs.HotStuffCore.Close()
		hs.manager.Close()
		hs.server.Server.Stop()
	})
}

// Propose broadcasts a new proposal to all replicas
func (hs *HotStuff) Propose() {
	proposal := hs.CreateProposal()
	logger.Debugf("Propose (%d commands): %s\n", len(proposal.Commands), proposal)
	protobuf := proto.BlockToProto(proposal)

	var ctx context.Context
	hs.proposeCancel()
	ctx, hs.proposeCancel = context.WithCancel(context.Background())
	hs.cfg.Propose(ctx, protobuf)
	// self-vote
	hs.handlePropose(proposal)
}

// SendNewView sends a NEW-VIEW message to a specific replica
func (hs *HotStuff) SendNewView(id config.ReplicaID) {
	qc := hs.GetQCHigh()
	if node, ok := hs.nodes[id]; ok {
		var ctx context.Context
		hs.newviewCancel()
		ctx, hs.newviewCancel = context.WithCancel(context.Background())
		node.NewView(ctx, proto.QuorumCertToProto(qc))
	}
}

func (hs *HotStuff) handlePropose(block *data.Block) {
	p, err := hs.OnReceiveProposal(block)
	if err != nil {
		logger.Info("OnReceiveProposal returned with error:", err)
		return
	}
	leaderID := hs.pacemaker.GetLeader(block.Height)
	if hs.Config.ID == leaderID {
		hs.OnReceiveVote(p)
	} else if leader, ok := hs.nodes[leaderID]; ok {
		var ctx context.Context
		hs.voteCancel()
		ctx, hs.voteCancel = context.WithCancel(context.Background())
		leader.Vote(ctx, proto.PartialCertToProto(p))
	}
}

type hotstuffServer struct {
	*HotStuff
	*gorums.Server
}

func newHotStuffServer(hs *HotStuff, srv *gorums.Server) *hotstuffServer {
	hsSrv := &hotstuffServer{
		HotStuff: hs,
		Server:   srv,
	}
	return hsSrv
}

func (hs *hotstuffServer) getClientID(ctx context.Context) (config.ReplicaID, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("getClientID: peerInfo not available")
	}

	if peerInfo.AuthInfo != nil && peerInfo.AuthInfo.AuthType() == "tls" {
		tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return 0, fmt.Errorf("getClientID: authInfo of wrong type: %T", peerInfo.AuthInfo)
		}
		if len(tlsInfo.State.PeerCertificates) > 0 {
			cert := tlsInfo.State.PeerCertificates[0]
			for replicaID := range hs.Config.Replicas {
				if subject, err := strconv.Atoi(cert.Subject.CommonName); err == nil && config.ReplicaID(subject) == replicaID {
					return replicaID, nil
				}
			}
		}
		return 0, fmt.Errorf("getClientID: could not find matching certificate")
	}

	// If we're not using TLS, we'll fallback to checking the metadata
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

	return config.ReplicaID(id), nil
}

// Propose handles a replica's response to the Propose QC from the leader
func (hs *hotstuffServer) Propose(ctx context.Context, protoB *proto.Block) {
	block := protoB.FromProto()
	id, err := hs.getClientID(ctx)
	if err != nil {
		logger.Infof("Failed to get client ID: %v", err)
		return
	}
	// defaults to 0 if error
	block.Proposer = id
	hs.handlePropose(block)
}

func (hs *hotstuffServer) Vote(ctx context.Context, cert *proto.PartialCert) {
	hs.OnReceiveVote(cert.FromProto())
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *hotstuffServer) NewView(ctx context.Context, msg *proto.QuorumCert) {
	qc := msg.FromProto()
	hs.OnReceiveNewView(qc)
}
