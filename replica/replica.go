// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/relab/hotstuff/clientsrv"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/invoker"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/synchronizer"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Replica is a participant in the consensus protocol.
type Replica struct {
	clientSrv *clientsrv.ClientServer
	cfg       *netconfig.Config
	hsSrv     *backend.Server
	hs        *core.Core
	invoker   *invoker.Invoker

	execHandlers map[clientsrv.CmdID]func(*emptypb.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}
}

// New returns a new replica.
func New(conf hotstuff.ReplicaConfig, builder core.Builder) (replica *Replica) {
	clientSrvOpts := conf.ClientServerOptions

	if conf.TLS {
		clientSrvOpts = append(clientSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(conf.Certificate)),
		))
	}

	clientSrv := clientsrv.NewClientServer(conf, clientSrvOpts)

	srv := &Replica{
		clientSrv:    clientSrv,
		execHandlers: make(map[clientsrv.CmdID]func(*emptypb.Empty, error)),
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

	srv.hsSrv = backend.NewServer(
		backend.WithLatencies(conf.ID, conf.Locations),
		backend.WithGorumsServerOptions(replicaSrvOpts...),
	)

	var creds credentials.TransportCredentials
	managerOpts := conf.ManagerOptions
	if conf.TLS {
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      conf.RootCAs,
			Certificates: []tls.Certificate{*conf.Certificate},
		})
	}
	srv.cfg = netconfig.NewConfig()

	// TODO(AlanRostem) check if this is enough to instantiate the invoker
	srv.invoker = invoker.New(creds, managerOpts...)
	builder.Add(
		srv.cfg,   // configuration
		srv.hsSrv, // event handling
		srv.invoker,

		srv.clientSrv,
		srv.clientSrv.CmdCache(),
	)
	srv.hs = builder.Build()

	return srv
}

// Modules returns the Modules object of this replica.
func (srv *Replica) Modules() *core.Core {
	return srv.hs
}

// StartServers starts the client and replica servers.
func (srv *Replica) StartServers(replicaListen, clientListen net.Listener) {
	srv.hsSrv.StartOnListener(replicaListen)
	srv.clientSrv.StartOnListener(clientListen)
}

// Connect connects to the other replicas.
func (srv *Replica) Connect(replicas []hotstuff.ReplicaInfo) error {
	return srv.invoker.Connect(replicas)
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

// Run runs the replica until the context is canceled.
func (srv *Replica) Run(ctx context.Context) {
	var (
		synchronizer *synchronizer.Synchronizer
		eventLoop    *core.EventLoop
	)
	srv.hs.Get(&synchronizer, &eventLoop)

	synchronizer.Start(ctx)
	eventLoop.Run(ctx)
}

// Close closes the connections and stops the servers used by the replica.
func (srv *Replica) Close() {
	srv.clientSrv.Stop()
	srv.invoker.Close()
	srv.hsSrv.Stop()
}

// GetHash returns the hash of all executed commands.
func (srv *Replica) GetHash() (b []byte) {
	return srv.clientSrv.Hash().Sum(b)
}

// GetCmdCount returns the count of all executed commands.
func (srv *Replica) GetCmdCount() (c uint32) {
	return srv.clientSrv.CmdCount()
}
