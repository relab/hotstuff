package replica

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	backend "github.com/relab/hotstuff/backend/gorums"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/consensus/fasthotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/synchronizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// cmdID is a unique identifier for a command
type cmdID struct {
	clientID    uint32
	sequenceNum uint64
}

// Config configures a replica.
type Config struct {
	// The id of the replica.
	ID consensus.ID
	// The private key of the replica.
	PrivateKey consensus.PrivateKey
	// Controls whether TLS is used.
	TLS bool
	// The TLS certificate.
	Certificate *tls.Certificate
	// The root certificates trusted by the replica.
	RootCAs *x509.CertPool
	// The name of the consensus implementation.
	Consensus string
	// The name of the crypto implementation.
	Crypto string
	// The name of the leader rotation algorithm.
	LeaderRotation string
	// The number of client commands that should be batched together in a block.
	BatchSize uint32
	// The number of blocks that should be cached.
	BlockCacheSize uint32
	// The timeout of the first view.
	InitialTimeout float64
	// The number of past views that should be used to calculate the next view duration.
	TimeoutSamples uint32
	// The number to multiply the view duration by in case of a view timeout.
	TimeoutMultiplier float64
	// Where to write command payloads.
	Output io.WriteCloser
	// Options for the client server.
	ClientServerOptions []gorums.ServerOption
	// Options for the replica server.
	ReplicaServerOptions []gorums.ServerOption
	// Options for the replica manager.
	ManagerOptions []gorums.ManagerOption
}

// Replica is a participant in the consensus protocol.
type Replica struct {
	output io.WriteCloser
	*clientSrv
	cfg      *backend.Config
	hsSrv    *backend.Server
	hs       *consensus.Modules
	cmdCache *cmdCache

	mut          sync.Mutex
	execHandlers map[cmdID]func(*empty.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}

	lastExecTime int64
}

// New returns a new replica.
func New(conf Config) (replica *Replica, err error) {
	clientSrvOpts := conf.ClientServerOptions

	if conf.TLS {
		clientSrvOpts = append(clientSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(conf.Certificate)),
		))
	}

	clientSrv, err := newClientServer(conf, clientSrvOpts)
	if err != nil {
		return nil, err
	}

	var consensusImpl consensus.Consensus
	switch conf.Consensus {
	case "chainedhotstuff":
		consensusImpl = chainedhotstuff.New()
	case "fasthotstuff":
		consensusImpl = fasthotstuff.New()
	default:
		fmt.Fprintf(os.Stderr, "Invalid consensus type: '%s'\n", conf.Consensus)
		os.Exit(1)
	}

	var cryptoImpl consensus.CryptoImpl
	switch conf.Crypto {
	case "ecdsa":
		cryptoImpl = ecdsa.New()
	case "bls12":
		cryptoImpl = bls12.New()
	default:
		fmt.Fprintf(os.Stderr, "Invalid crypto type: '%s'\n", conf.Crypto)
		os.Exit(1)
	}

	var leaderRotation consensus.LeaderRotation
	switch conf.LeaderRotation {
	case "round-robin":
		leaderRotation = leaderrotation.NewRoundRobin()
	case "fixed":
		// TODO: consider making this configurable.
		leaderRotation = leaderrotation.NewFixed(1)
	}

	srv := &Replica{
		clientSrv:    clientSrv,
		cmdCache:     newCmdCache(int(conf.BatchSize)),
		execHandlers: make(map[cmdID]func(*empty.Empty, error)),
		output:       conf.Output,
		lastExecTime: time.Now().UnixNano(),
		cancel:       func() {},
		done:         make(chan struct{}),
	}

	replicaSrvOpts := conf.ReplicaServerOptions
	if conf.TLS {
		replicaSrvOpts = append(replicaSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{*conf.Certificate},
				ClientCAs:    conf.RootCAs,
				ClientAuth:   tls.RequireAndVerifyClientCert,
			})),
		))
	}

	srv.hsSrv = backend.NewServer(replicaSrvOpts...)

	managerOpts := conf.ManagerOptions
	if conf.TLS {
		managerOpts = append(managerOpts, gorums.WithGrpcDialOptions(grpc.WithTransportCredentials(
			credentials.NewTLS(&tls.Config{
				RootCAs:      conf.RootCAs,
				Certificates: []tls.Certificate{*conf.Certificate},
			}),
		)))
	}
	srv.cfg = backend.NewConfig(conf.ID, managerOpts...)

	builder := consensus.NewBuilder(conf.ID, conf.PrivateKey)

	builder.Register(
		srv.cfg,
		srv.hsSrv,
		consensusImpl,
		crypto.NewCache(cryptoImpl, 100), // TODO: consider making this configurable
		leaderRotation,
		srv,          // executor
		srv.cmdCache, // acceptor and command queue
		synchronizer.New(synchronizer.NewViewDuration(
			uint64(conf.TimeoutSamples), float64(conf.InitialTimeout), float64(conf.TimeoutMultiplier)),
		),
		blockchain.New(int(conf.BlockCacheSize)),
		logging.New(fmt.Sprintf("hs%d", conf.ID)),
	)
	srv.hs = builder.Build()

	return srv, nil
}

// StartServers starts the client and replica servers.
func (srv *Replica) StartServers(replicaListen, clientListen net.Listener) {
	srv.hsSrv.StartOnListener(replicaListen)
	srv.clientSrv.StartOnListener(clientListen)
}

// Connect connects to the other replicas.
func (srv *Replica) Connect(replicas *config.ReplicaConfig) error {
	return srv.cfg.Connect(replicas)
}

// Start runs the replica in a goroutine.
func (srv *Replica) Start() {
	var ctx context.Context
	ctx, srv.cancel = context.WithCancel(context.Background())
	go func() {
		srv.Run(ctx)
		close(srv.done)
	}()
}

// Stop stops the replica and closes connections.
func (srv *Replica) Stop() {
	srv.cancel()
	<-srv.done
	srv.Close()
}

// Run runs the replica until the context is cancelled.
func (srv *Replica) Run(ctx context.Context) {
	srv.hs.Synchronizer().Start(ctx)
	srv.hs.EventLoop().Run(ctx)
}

// Close closes the connections and stops the servers used by the replica.
func (srv *Replica) Close() error {
	srv.clientSrv.Stop()
	srv.cfg.Close()
	srv.hsSrv.Stop()
	return srv.output.Close()
}
