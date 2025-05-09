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
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type replicaOptions struct {
	isSecure      bool
	useKauri      bool
	clientServer  gorums.ServerOption
	replicaServer gorums.ServerOption
	credentials   credentials.TransportCredentials
}

type ReplicaOption func(*replicaOptions)

func WithTLS(certificate tls.Certificate, rootCAs *x509.CertPool) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.isSecure = true
		creds := credentials.NewTLS(&tls.Config{
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{certificate},
		})
		ro.clientServer = gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(&certificate)),
		)
		ro.replicaServer = gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{certificate},
			ClientCAs:    rootCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})))
		ro.credentials = creds
	}
}

func WithKauri(tree tree.Tree) ReplicaOption {
	return func(ro *replicaOptions) {

	}
}

type ReplicaDependencies struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	options      *core.Options
}

// TODO(AlanRostem): decouple the majority of dependency creation and pass dependency sets instead
func NewReplicaDependencies(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	repOpts ...ReplicaOption,
) (*ReplicaDependencies, error) {
	var rOpt replicaOptions
	for _, opt := range repOpts {
		opt(&rOpt)
	}

	id := opts.HotstuffID()
	seed := opts.GetSharedSeed()

	depsCore := dependencies.NewCore(id, "hs", privKey)
	depsCore.Options.SetSharedRandomSeed(seed)

	// TODO(AlanRostem): make this case into an option
	if opts.GetKauri() {
		delayMode := tree.DelayTypeNone
		if opts.GetAggregationTime() {
			delayMode = tree.DelayTypeAggregation
		}

		// create tree only if we are using tree leader (Kauri)
		t := tree.NewDelayed(
			opts.HotstuffID(),
			delayMode,
			int(opts.GetBranchFactor()),
			latency.MatrixFrom(opts.GetLocations()),
			opts.TreePositionIDs(),
			opts.GetTreeDelta().AsDuration(),
		)
		depsCore.Options.SetTree(t)
	}

	depsNet := dependencies.NewNetwork(
		depsCore,
		rOpt.credentials,
	)
	cacheSize := 100 // TODO: consider making this configurable
	depsSecure, err := dependencies.NewSecurity(depsCore, depsNet, opts.GetCrypto(), cacheSize)
	if err != nil {
		return nil, err
	}
	depsSrv := dependencies.NewService(
		depsCore,
		depsSecure,
		int(opts.GetBatchSize()),
		rOpt.clientServer)

	durationOpts := viewduration.NewOptions(
		opts.GetTimeoutSamples(),
		opts.GetInitialTimeout().AsDuration(),
		opts.GetMaxTimeout().AsDuration(),
		opts.GetTimeoutMultiplier(),
	)
	depsProtocol, err := dependencies.NewProtocol(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		opts.GetKauri(),
		opts.GetConsensus(),
		opts.GetLeaderRotation(),
		opts.GetByzantineStrategy(),
		durationOpts)
	if err != nil {
		return nil, err
	}
	serverSrvOpt := []server.ServerOptions{
		server.WithLatencies(opts.HotstuffID(), opts.GetLocations()),
		server.WithGorumsServerOptions(rOpt.replicaServer),
	}
	if opts.GetKauri() {
		serverSrvOpt = append(serverSrvOpt, server.WithKauri())
	}
	server := server.NewServer(
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
		depsNet.Config,
		depsSecure.BlockChain,
		serverSrvOpt...,
	)

	return &ReplicaDependencies{
		eventLoop:    depsCore.EventLoop,
		logger:       depsCore.Logger,
		options:      depsCore.Options,
		sender:       depsNet.Sender,
		clientSrv:    depsSrv.ClientSrv,
		synchronizer: depsProtocol.Synchronizer,
		server:       server,
	}, nil
}
