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
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type replicaOptions struct {
	isSecure          bool
	clientServerOpts  []gorums.ServerOption
	replicaServerOpts []gorums.ServerOption
	credentials       credentials.TransportCredentials

	useKauri  bool
	kauriTree tree.Tree
}

type ReplicaOption func(*replicaOptions)

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

func WithKauri(tree tree.Tree) ReplicaOption {
	return func(ro *replicaOptions) {
		ro.useKauri = true
		ro.kauriTree = tree
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

// TODO(AlanRostem): decouple the majority of dependency creation and pass dependency sets instead
func NewReplicaDependencies(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	vdParams viewduration.Params,
	repOpts ...ReplicaOption,
) (*ReplicaDependencies, error) {
	var rOpt replicaOptions
	for _, opt := range repOpts {
		opt(&rOpt)
	}

	if !rOpt.isSecure {
		rOpt.credentials = insecure.NewCredentials()
	}

	// TODO(AlanRostem): remove direct dependency on these values. I am putting these here to visualize the dependency on *orchestrationpb.ReplicaOpts for now.
	id := opts.HotstuffID()
	seed := opts.GetSharedSeed()
	locations := opts.GetLocations()

	cryptoName := opts.GetCrypto()

	globalOpts := []core.GlobalsOption{
		core.WithSharedRandomSeed(seed),
	}

	if rOpt.useKauri {
		globalOpts = append(globalOpts, core.WithKauri(&rOpt.kauriTree))
	}

	depsCore := dependencies.NewCore(id, "hs", privKey, globalOpts...)

	depsNet := dependencies.NewNetwork(
		depsCore,
		rOpt.credentials,
	)
	cacheSize := 100 // TODO: consider making this configurable
	depsSecure, err := dependencies.NewSecurity(depsCore, depsNet, cryptoName, cacheSize)
	if err != nil {
		return nil, err
	}
	depsSrv := dependencies.NewService(
		depsCore,
		depsSecure,
		int(opts.GetBatchSize()),
		rOpt.clientServerOpts...)

	depsProtocol, err := dependencies.NewProtocol(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		opts.GetConsensus(),
		opts.GetLeaderRotation(),
		opts.GetByzantineStrategy(),
		vdParams,
	)
	if err != nil {
		return nil, err
	}
	srvOpt := []server.ServerOptions{
		server.WithLatencies(id, locations),
		server.WithGorumsServerOptions(rOpt.replicaServerOpts...),
	}
	server := server.NewServer(
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Globals,
		depsNet.Config,
		depsSecure.BlockChain,
		srvOpt...,
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
