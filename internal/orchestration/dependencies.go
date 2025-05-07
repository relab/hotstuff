package orchestration

import (
	"crypto/tls"
	"crypto/x509"
	"time"

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
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type ReplicaDependencies struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	options      *core.Options
}

func NewReplicaDependencies(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*ReplicaDependencies, error) {
	var creds credentials.TransportCredentials
	clientSrvOpts := []gorums.ServerOption{}
	replicaSrvOpts := []gorums.ServerOption{}
	if opts.GetUseTLS() {
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{certificate},
		})

		clientSrvOpts = append(clientSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewServerTLSFromCert(&certificate)),
		))

		replicaSrvOpts = append(replicaSrvOpts, gorums.WithGRPCServerOptions(
			grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{certificate},
				ClientCAs:    rootCAs,
				ClientAuth:   tls.RequireAndVerifyClientCert,
			})),
		))
	}

	depsCore := dependencies.NewCore(opts.HotstuffID(), "hs", privKey)
	depsCore.Options.SetSharedRandomSeed(opts.GetSharedSeed())

	if opts.GetKauri() {
		// TODO(meling): Temporary default; should be configurable and moved to the appropriate place.
		opts.SetTreeHeightWaitTime()
		// create tree only if we are using tree leader (Kauri)
		t := createTree(opts)
		depsCore.Options.SetTree(t)
	}

	depsNet := dependencies.NewNetwork(
		depsCore,
		creds,
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
		clientSrvOpts)
	durationOpts := dependencies.ViewDurationOptions{
		SampleSize:   uint64(opts.GetTimeoutSamples()),
		StartTimeout: float64(opts.GetInitialTimeout().AsDuration().Nanoseconds()) / float64(time.Millisecond),
		MaxTimeout:   float64(opts.GetMaxTimeout().AsDuration().Nanoseconds()) / float64(time.Millisecond),
		Multiplier:   float64(opts.GetTimeoutMultiplier()),
	}

	// TODO(AlanRostem): find a cleaner way to do this.
	// This is needed for the tree-leader to work properly.
	durationOpts.IsFixed = opts.GetLeaderRotation() == leaderrotation.TreeLeaderModuleName

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
	server := server.NewServer(
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
		depsNet.Config,
		depsSecure.BlockChain,

		server.WithLatencies(opts.HotstuffID(), opts.GetLocations()),
		server.WithGorumsServerOptions(replicaSrvOpts...),
	)
	if opts.GetKauri() {
		// TODO: cannot check if Kauri is enabled when hiding protocol module initialization. Find a workaround.
		// if modsProtocol.Kauri == nil {
		// 	return nil, fmt.Errorf("kauri was enabled but its module was not initialized")
		// }
		kauri.RegisterService(depsCore.EventLoop, depsCore.Logger, server)
	}

	return &ReplicaDependencies{
		clientSrv:    depsSrv.ClientSrv,
		server:       server,
		sender:       depsNet.Sender,
		synchronizer: depsProtocol.Synchronizer,
		eventLoop:    depsCore.EventLoop,
		logger:       depsCore.Logger,
		options:      depsCore.Options,
	}, nil
}

// createTree creates a tree based on the given replica options.
func createTree(replicaOpts *orchestrationpb.ReplicaOpts) tree.Tree {
	tree := tree.CreateTree(replicaOpts.HotstuffID(), int(replicaOpts.GetBranchFactor()), replicaOpts.TreePositionIDs())
	switch {
	case replicaOpts.GetAggregationTime():
		tree.SetAggregationWaitTime(latency.MatrixFrom(replicaOpts.GetLocations()), replicaOpts.TreeDeltaDuration())
	case replicaOpts.GetTreeHeightTime():
		fallthrough
	default:
		tree.SetTreeHeightWaitTime(replicaOpts.TreeDeltaDuration())
	}
	return tree
}
