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
	"github.com/relab/hotstuff/service/server"
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
	rules, err := wiring.NewConsensusRules(
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
			rules,
			byzStrat,
		)
		if err != nil {
			return nil, err
		}
		rules = byz
		depsCore.Logger().Infof("assigned byzantine strategy: %s", byzStrat)
	}
	depsSrv := wiring.NewService(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsSecure.BlockChain(),
		rules,
		rOpt.cmdCacheOpts,
		rOpt.clientGorumsSrvOpts...,
	)
	leader, err := wiring.NewLeaderRotation(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSrv.Committer(),
		vdParams,
		rOpt.moduleNames.leaderRotation,
		rules.ChainLength(),
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
	var protocol modules.ConsensusProtocol
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
			leader,
			sender,
		)
	}
	depsConsensus := wiring.NewConsensus(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.Authority(),
		depsSrv.CmdCache(),
		depsSrv.Committer(),
		rules,
		leader,
		protocol,
	)
	// TODO(AlanRostem): consder a way to move the consensus flow from Synchronzier to Consensus
	synchronizer := synchronizer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.Authority(),
		leader,
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
		clientSrv:    depsSrv.ClientSrv(),
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
