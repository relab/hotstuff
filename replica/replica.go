// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"net"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/dependencies"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/viewstates"
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
	sender       *network.GorumsSender
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
	sender := network.NewGorumsSender(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		rOpt.credentials,
	)
	depsSecure, err := dependencies.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		names.crypto,
		certauth.WithCache(100), // TODO: consider making this configurable
	)
	if err != nil {
		return nil, err
	}
	rules, err := dependencies.NewConsensusRules(
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
		byz, err := dependencies.WrapByzantineStrategy(
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
	depsSrv := dependencies.NewService(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsSecure.BlockChain(),
		rules,
		rOpt.cmdCacheOpts,
		rOpt.clientGorumsSrvOpts...,
	)
	leader, err := dependencies.NewLeaderRotation(
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
	viewStates := viewstates.New(
		depsCore.Logger(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	var protocol modules.ConsensusProtocol
	if depsCore.RuntimeCfg().KauriEnabled() {
		protocol = kauri.New(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.CertAuth(),
			kauri.NewExtendedGorumsSender(
				depsCore.EventLoop(),
				depsCore.Logger(),
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
			depsSecure.CertAuth(),
			viewStates,
			leader,
			sender,
		)
	}
	depsConsensus := dependencies.NewConsensus(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.CertAuth(),
		depsSrv.CmdCache(),
		rules,
		leader,
	)
	// TODO(AlanRostem): explore ways to simplify consensus and synchronizer so that they take in less dependencies.
	consensus := consensus.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		protocol,
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		depsSrv.CmdCache(),
		depsSrv.Committer(),
	)
	// TODO(AlanRostem): consder a way to move the consensus flow from Synchronzier to Consensus
	synchronizer := synchronizer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.CertAuth(),
		leader,
		consensus,
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
