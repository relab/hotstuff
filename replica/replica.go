// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"net"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/dependencies"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/cmdcache"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Replica is a participant in the consensus protocol.
type Replica struct {
	eventLoop    *eventloop.EventLoop
	clientSrv    *clientsrv.Server
	hsSrv        *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer

	execHandlers map[cmdcache.CmdID]func(*emptypb.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}
}

// New returns a new replica.
func New(
	depsCore *dependencies.Core,
	vdParams viewduration.Params,
	opts ...Option,
) (replica *Replica, err error) {
	rOpt := newDefaultOpts()
	names := &rOpt.moduleNames
	for _, opt := range opts {
		opt(rOpt)
	}
	depsNet := dependencies.NewNetwork(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.Globals(),
		rOpt.credentials,
	)
	depsSecure, err := dependencies.NewSecurity(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.Globals(),
		depsNet.Sender(),
		names.crypto,
		certauth.WithCache(100), // TODO: consider making this configurable
	)
	if err != nil {
		return nil, err
	}
	depsSrv := dependencies.NewService(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsSecure.BlockChain(),
		rOpt.cmdCacheOpts,
		rOpt.clientGorumsSrvOpts...,
	)
	depsProtocol, err := dependencies.NewProtocol(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		names.consensus,
		names.leaderRotation,
		names.byzantineStrategy,
		vdParams,
	)
	if err != nil {
		return nil, err
	}
	rOpt.serverOpts = append(rOpt.serverOpts, server.WithGorumsServerOptions(rOpt.replicaGorumsSrvOpts...))
	server := server.NewServer(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.Globals(),
		depsSecure.BlockChain(),
		rOpt.serverOpts...,
	)
	srv := &Replica{
		eventLoop:    depsCore.EventLoop(),
		clientSrv:    depsSrv.ClientSrv(),
		sender:       depsNet.Sender(),
		synchronizer: depsProtocol.Synchronizer(),
		hsSrv:        server,

		execHandlers: make(map[cmdcache.CmdID]func(*emptypb.Empty, error)),
		cancel:       func() {},
		done:         make(chan struct{}),
	}
	return srv, nil
}

// StartServers starts the client and replica servers.
func (srv *Replica) StartServers(replicaListen, clientListen net.Listener) {
	srv.hsSrv.StartOnListener(replicaListen)
	srv.clientSrv.StartOnListener(clientListen)
}

// Connect connects to the other replicas.
func (srv *Replica) Connect(replicas []hotstuff.ReplicaInfo) error {
	return srv.sender.Connect(replicas)
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
	srv.synchronizer.Start(ctx)
	srv.eventLoop.Run(ctx)
}

// Close closes the connections and stops the servers used by the replica.
func (srv *Replica) Close() {
	srv.clientSrv.Stop()
	srv.sender.Close()
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
