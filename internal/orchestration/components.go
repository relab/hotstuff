package orchestration

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/orchestration/components"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/committer"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type CoreComponents struct {
	Options   *core.Options
	EventLoop *eventloop.EventLoop
	Logger    logging.Logger
}

func NewCoreComponents(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
) *CoreComponents {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &CoreComponents{
		Options:   core.NewOptions(id, privKey),
		EventLoop: eventloop.New(logger, 100),
		Logger:    logger,
	}
}

type NetworkComponents struct {
	Config *netconfig.Config
	Sender *sender.Sender
}

func NewNetworkComponents(
	coreComps *CoreComponents,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *NetworkComponents {
	cfg := netconfig.NewConfig()
	send := sender.New(
		cfg,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
		creds,
		mgrOpts...,
	)

	return &NetworkComponents{
		Config: cfg,
		Sender: send,
	}
}

type SecurityComponents struct {
	BlockChain *blockchain.BlockChain
	CryptoImpl modules.CryptoBase
	CertAuth   *certauth.CertAuthority
}

func NewSecurityComponents(
	coreComps *CoreComponents,
	netComps *NetworkComponents,
	cryptoName string,
	cacheSize int,
) (*SecurityComponents, error) {
	blockChain := blockchain.New(
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
	)
	cryptoImpl, err := components.NewCryptoImpl(cryptoName, netComps.Config, coreComps.Logger, coreComps.Options)
	if err != nil {
		return nil, err
	}
	var certAuthority *certauth.CertAuthority
	if cacheSize > 0 {
		certAuthority = certauth.NewCached(
			cryptoImpl,
			blockChain,
			coreComps.Logger,
			cacheSize,
		)
	} else {
		certAuthority = certauth.New(
			cryptoImpl,
			blockChain,
			coreComps.Logger,
		)
	}
	return &SecurityComponents{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth:   certAuthority,
	}, nil
}

type ServiceComponents struct {
	CmdCache  *clientsrv.CmdCache
	ClientSrv *clientsrv.ClientServer
	Committer *committer.Committer
}

func NewServiceComponents(
	coreComps *CoreComponents,
	secureComps *SecurityComponents,
	batchSize int,
	clientSrvOpts []gorums.ServerOption,
) *ServiceComponents {
	cmdCache := clientsrv.NewCmdCache(
		coreComps.Logger,
		batchSize,
	)
	clientSrv := clientsrv.NewClientServer(
		coreComps.EventLoop,
		coreComps.Logger,
		cmdCache,
		clientSrvOpts,
	)
	committer := committer.New(
		secureComps.BlockChain,
		clientSrv,
		coreComps.Logger,
	)
	return &ServiceComponents{
		CmdCache:  cmdCache,
		ClientSrv: clientSrv,
		Committer: committer,
	}
}

type ProtocolModules struct {
	ConsensusRules modules.ConsensusRules
	Kauri          modules.Kauri
	LeaderRotation modules.LeaderRotation
	ViewDuration   modules.ViewDuration
}

type ViewDurationOptions struct {
	SampleSize   uint64
	StartTimeout float64
	MaxTimeout   float64
	Multiplier   float64
}

func NewProtocolModules(
	coreComps *CoreComponents,
	netComps *NetworkComponents,
	secureComps *SecurityComponents,
	srvComps *ServiceComponents,

	opts *orchestrationpb.ReplicaOpts, // TODO: avoid modifying this so it doesn't depend on orchestrationpb
	consensusName, leaderRotationName, byzantineStrategy string,
	vdOpt ViewDurationOptions,
) (*ProtocolModules, error) {
	consensusRules, err := components.NewConsensusRules(consensusName, secureComps.BlockChain, coreComps.Logger, coreComps.Options)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := components.NewLeaderRotation(
		leaderRotationName,
		consensusRules.ChainLength(),
		secureComps.BlockChain,
		netComps.Config,
		srvComps.Committer,
		coreComps.Logger,
		coreComps.Options,
	)
	if err != nil {
		return nil, err
	}

	var kauriOptional modules.Kauri = nil

	if opts.GetKauri() {
		kauriOptional = kauri.New(
			secureComps.CryptoImpl,
			leaderRotation,
			secureComps.BlockChain,
			coreComps.Options,
			coreComps.EventLoop,
			netComps.Config,
			netComps.Sender,
			coreComps.Logger,
		)
	}

	var duration modules.ViewDuration
	if leaderRotationName == leaderrotation.TreeLeaderModuleName {
		// TODO(meling): Temporary default; should be configurable and moved to the appropriate place.
		opts.SetTreeHeightWaitTime()
		// create tree only if we are using tree leader (Kauri)
		coreComps.Options.SetTree(createTree(opts))
		duration = synchronizer.NewFixedViewDuration(opts.GetInitialTimeout().AsDuration())
	} else {
		duration = synchronizer.NewViewDuration(
			vdOpt.SampleSize, vdOpt.StartTimeout, vdOpt.MaxTimeout, vdOpt.Multiplier,
		)
	}
	if byzantineStrategy != "" {
		byz, err := components.NewByzantineStrategy(
			byzantineStrategy,
			consensusRules,
			secureComps.BlockChain,
			coreComps.Options)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		coreComps.Logger.Infof("assigned byzantine strategy: %s", byzantineStrategy)
	}
	return &ProtocolModules{
		ConsensusRules: consensusRules,
		Kauri:          kauriOptional,
		LeaderRotation: leaderRotation,
		ViewDuration:   duration,
	}, nil
}

type ProtocolComponents struct {
	Consensus    *consensus.Consensus
	Synchronizer *synchronizer.Synchronizer
}

func NewProtocolComponents(
	coreComps *CoreComponents,
	netComps *NetworkComponents,
	secureComps *SecurityComponents,
	srvComps *ServiceComponents,
	mods *ProtocolModules,
) *ProtocolComponents {
	csus := consensus.New(
		mods.ConsensusRules,
		mods.LeaderRotation,
		mods.Kauri,
		secureComps.BlockChain,
		srvComps.Committer,
		srvComps.CmdCache,
		netComps.Sender,
		secureComps.CertAuth,
		netComps.Config,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
	)
	synch := synchronizer.New(
		secureComps.CryptoImpl,
		mods.LeaderRotation,
		mods.ViewDuration,
		secureComps.BlockChain,
		csus,
		secureComps.CertAuth,
		netComps.Config,
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
	)
	return &ProtocolComponents{
		Consensus:    csus,
		Synchronizer: synch,
	}
}

type ReplicaComponents struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	options      *core.Options
}

func NewReplicaComponents(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*ReplicaComponents, error) {
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

	coreComps := NewCoreComponents(opts.HotstuffID(), "hs", privKey)
	coreComps.Options.SetSharedRandomSeed(opts.GetSharedSeed())
	// TODO: Upon a merge with master, this doesn't compile.
	// coreComps.Options.SetTreeConfig(opts.GetBranchFactor(), opts.TreePositionIDs(), opts.TreeDeltaDuration())

	netComps := NewNetworkComponents(
		coreComps,
		creds,
	)
	cacheSize := 100 // TODO: consider making this configurable
	secureComps, err := NewSecurityComponents(coreComps, netComps, opts.GetCrypto(), cacheSize)
	if err != nil {
		return nil, err
	}
	srvComps := NewServiceComponents(coreComps, secureComps, int(opts.GetBatchSize()), clientSrvOpts)

	durationOpts := ViewDurationOptions{
		SampleSize:   uint64(opts.GetTimeoutSamples()),
		StartTimeout: float64(opts.GetInitialTimeout().AsDuration().Nanoseconds()) / float64(time.Millisecond),
		MaxTimeout:   float64(opts.GetMaxTimeout().AsDuration().Nanoseconds()) / float64(time.Millisecond),
		Multiplier:   float64(opts.GetTimeoutMultiplier()),
	}
	protocolMods, err := NewProtocolModules(
		coreComps, netComps, secureComps, srvComps, opts,
		opts.GetConsensus(), opts.GetLeaderRotation(), opts.GetByzantineStrategy(),
		durationOpts)
	if err != nil {
		return nil, err
	}
	protocolComps := NewProtocolComponents(coreComps, netComps, secureComps, srvComps, protocolMods)
	server := server.NewServer(
		secureComps.BlockChain,
		netComps.Config,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,

		server.WithLatencies(opts.HotstuffID(), opts.GetLocations()),
		server.WithGorumsServerOptions(replicaSrvOpts...),
	)
	if opts.GetKauri() {
		if protocolMods.Kauri == nil {
			return nil, fmt.Errorf("kauri was enabled but its module was not initialized")
		}
		kauri.RegisterService(coreComps.EventLoop, coreComps.Logger, server)
	}

	return &ReplicaComponents{
		clientSrv:    srvComps.ClientSrv,
		server:       server,
		sender:       netComps.Sender,
		synchronizer: protocolComps.Synchronizer,
		eventLoop:    coreComps.EventLoop,
		logger:       coreComps.Logger,
		options:      coreComps.Options,
	}, nil
}
