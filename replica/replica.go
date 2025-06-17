// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"net"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/wiring"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/server"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Replica is a participant in the consensus protocol.
type Replica struct {
	eventLoop    *eventloop.EventLoop
	clientSrv    *clientpb.Server
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
	vdParams viewduration.Params,
	opts ...Option,
) (replica *Replica, err error) {
	rOpt := newDefaultOpts()
	names := &rOpt.moduleNames
	for _, opt := range opts {
		opt(rOpt)
	}
	sender := network.NewGorumsSender(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		rOpt.credentials,
	)
	depsSecure, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		names.crypto,
		cert.WithCache(100), // TODO: consider making this configurable
	)
	if err != nil {
		return nil, err
	}
	consensusRules, err := wiring.NewConsensusRules(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		rOpt.moduleNames.consensus,
	)
	if err != nil {
		return nil, err
	}

	byzStrat := rOpt.moduleNames.byzantineStrategy
	if byzStrat != "" {
		byz, err := wiring.WrapByzantineStrategy(
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			consensusRules,
			byzStrat,
		)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		depsCore.Logger().Infof("assigned byzantine strategy: %s", byzStrat)
	}
	depsClient := wiring.NewClient(
		depsCore.EventLoop(),
		depsCore.Logger(),
		rOpt.cmdCacheOpts,
		rOpt.clientGorumsSrvOpts...,
	)
	committer := committer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecure.BlockChain(),
		consensusRules,
	)
	leaderRotation, err := wiring.NewLeaderRotation(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		committer,
		rOpt.moduleNames.leaderRotation,
		consensusRules.ChainLength(),
	)
	if err != nil {
		return nil, err
	}
	// TODO(AlanRostem): avoid creating viewstates here.
	viewStates, err := consensus.NewViewStates(
		depsSecure.BlockChain(),
		depsSecure.Authority(),
	)
	if err != nil {
		return nil, err
	}
	var protocol modules.DisseminatorAggregator
	if depsCore.RuntimeCfg().HasKauriTree() {
		protocol = kauri.New(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.Authority(),
			kauri.NewExtendedGorumsSender(
				depsCore.EventLoop(),
				depsCore.RuntimeCfg(),
				sender,
			),
		)
	} else {
		protocol = consensus.NewHotStuff(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.Authority(),
			viewStates,
			leaderRotation,
			sender,
		)
	}
	depsConsensus := wiring.NewConsensus(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.Authority(),
		depsClient.Cache(),
		committer,
		consensusRules,
		leaderRotation,
		protocol,
	)
	viewDuration := viewduration.NewDynamic(vdParams) // TODO(AlanRostem): should be fixed when selecting kauri
	// TODO(AlanRostem): consder moving the consensus flow from Synchronzier to a different class
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
