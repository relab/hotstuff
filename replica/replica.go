// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"net"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/wiring"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/server"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Replica is a participant in the consensus protocol.
type Replica struct {
	eventLoop    *eventloop.EventLoop
	clientSrv    *server.ClientIO
	hsSrv        *server.Server
	sender       *network.GorumsSender
	synchronizer *synchronizer.Synchronizer

	execHandlers map[clientpb.MessageID]func(*emptypb.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}
}

// New returns a new replica.
func New(
	depsCore *wiring.Core,
	depsSecure *wiring.Security,
	sender *network.GorumsSender,
	viewStates *protocol.ViewStates,
	comm comm.Communication,
	leaderRotation leaderrotation.LeaderRotation,
	consensusRules consensus.Ruleset,
	viewDuration synchronizer.ViewDuration,
	commandBatchSize uint32,
	opts ...Option,
) (replica *Replica, err error) {
	rOpt := newDefaultOpts()
	for _, opt := range opts {
		opt(rOpt)
	}
	depsClient := wiring.NewClient(
		depsCore.EventLoop(),
		depsCore.Logger(),
		commandBatchSize,
		rOpt.clientGorumsSrvOpts...,
	)
	depsConsensus := wiring.NewConsensus(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.Authority(),
		depsClient.Cache(),
		consensusRules,
		leaderRotation,
		viewStates,
		comm,
	)
	synchronizer := synchronizer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.Authority(),
		leaderRotation,
		viewDuration,
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		viewStates,
		sender,
	)
	rOpt.serverOpts = append(rOpt.serverOpts, server.WithGorumsServerOptions(rOpt.replicaGorumsSrvOpts...))
	server := server.NewServer(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		rOpt.serverOpts...,
	)
	srv := &Replica{
		eventLoop:    depsCore.EventLoop(),
		clientSrv:    depsClient.Server(),
		sender:       sender,
		synchronizer: synchronizer,
		hsSrv:        server,

		execHandlers: make(map[clientpb.MessageID]func(*emptypb.Empty, error)),
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
