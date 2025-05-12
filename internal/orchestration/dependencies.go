package orchestration

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/dependencies"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type moduleNames struct {
	crypto,
	consensus,
	leaderRotation,
	byzantineStrategy string
}

type replicaOptions struct {
	isSecure          bool
	credentials       credentials.TransportCredentials
	clientServerOpts  []gorums.ServerOption
	replicaServerOpts []gorums.ServerOption
	globalOpts        []core.GlobalOption
	cmdCacheOpts      []cmdcache.Option
	serverOpts        []server.ServerOption
	moduleNames       moduleNames
}

type ReplicaOption func(*replicaOptions)

// OverrideCrypto changes the crypto implementation to the one specified by name.
// Default: "ecdsa"
func OverrideCrypto(moduleName string) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.moduleNames.crypto = moduleName
	}
}

// OverrideCrypto changes the consensus rules to the one specified by name.
// Default: "chainedhotstuff"
func OverrideConsensusRules(moduleName string) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.moduleNames.consensus = moduleName
	}
}

// OverrideCrypto changes the leader rotation scheme to the one specified by name.
// Default: "round-robin"
func OverrideLeaderRotation(moduleName string) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.moduleNames.leaderRotation = moduleName
	}
}

// WithByzantineStrategy wraps the existing consensus rules to a byzantine ruleset specified by name.
func WithByzantineStrategy(strategyName string) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.moduleNames.byzantineStrategy = strategyName
	}
}

func WithGlobalOptions(opts ...core.GlobalOption) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.globalOpts = append(ro.globalOpts, opts...)
	}
}

func WithCmdCacheOptions(opts ...cmdcache.Option) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.cmdCacheOpts = append(ro.cmdCacheOpts, opts...)
	}
}

func WithServerOptions(opts ...server.ServerOption) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.serverOpts = append(ro.serverOpts, opts...)
	}
}

func WithTLS(certificate tls.Certificate, rootCAs *x509.CertPool) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.isSecure = true
		creds := credentials.NewTLS(&tls.Config{
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{certificate},
		})
		ro.clientServerOpts = append(ro.clientServerOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(&certificate)),
		))
		ro.replicaServerOpts = append(ro.replicaServerOpts, gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{certificate},
			ClientCAs:    rootCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}))))
		ro.credentials = creds
	}
}

type ReplicaDependencies struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	options      *core.Globals
}

// TODO(AlanRostem): put this in "dependencies" package
func NewReplicaDependencies(
	id hotstuff.ID,
	privKey hotstuff.PrivateKey,
	vdParams viewduration.Params,
	repOpts ...ReplicaOption,
) (*ReplicaDependencies, error) {
	var rOpt replicaOptions
	names := &rOpt.moduleNames
	*names = moduleNames{
		crypto:            ecdsa.ModuleName,
		consensus:         chainedhotstuff.ModuleName,
		leaderRotation:    leaderrotation.RoundRobinModuleName,
		byzantineStrategy: "",
	}

	for _, opt := range repOpts {
		opt(&rOpt)
	}
	if !rOpt.isSecure {
		rOpt.credentials = insecure.NewCredentials()
	}
	depsCore := dependencies.NewCore(id, "hs", privKey, rOpt.globalOpts...)
	depsNet := dependencies.NewNetwork(
		depsCore,
		rOpt.credentials,
	)
	depsSecure, err := dependencies.NewSecurity(
		depsCore,
		depsNet,
		names.crypto,
		certauth.WithCache(100), // TODO: consider making this configurable
	)
	if err != nil {
		return nil, err
	}
	depsSrv := dependencies.NewService(
		depsCore,
		depsSecure,
		rOpt.cmdCacheOpts,
		rOpt.clientServerOpts...,
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
	rOpt.serverOpts = append(rOpt.serverOpts, server.WithGorumsServerOptions(rOpt.replicaServerOpts...))
	server := server.NewServer(
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Globals,
		depsNet.Config,
		depsSecure.BlockChain,
		rOpt.serverOpts...,
	)
	return &ReplicaDependencies{
		eventLoop:    depsCore.EventLoop,
		logger:       depsCore.Logger,
		options:      depsCore.Globals,
		sender:       depsNet.Sender,
		clientSrv:    depsSrv.ClientSrv,
		synchronizer: depsProtocol.Synchronizer,
		server:       server,
	}, nil
}
