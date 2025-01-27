package orchestration

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/certauth"
	"github.com/relab/hotstuff/clientsrv"
	"github.com/relab/hotstuff/committer"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/byzantine"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/consensus/fasthotstuff"
	"github.com/relab/hotstuff/consensus/simplehotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/eddsa"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/kauri"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/sender"
	"github.com/relab/hotstuff/server"
	"github.com/relab/hotstuff/synchronizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func getConsensusRules(
	name string,
	blockChain *blockchain.BlockChain,
	logger logging.Logger,
	opts *core.Options,
) (rules modules.Rules, ok bool) {
	ok = true
	switch name {
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(blockChain, logger, opts)
	case simplehotstuff.ModuleName:
		rules = simplehotstuff.New(blockChain, logger)
	case chainedhotstuff.ModuleName:
		rules = chainedhotstuff.New(blockChain, logger)
	default:
		return nil, false
	}
	return
}

func getByzantine(
	name string,
	rules modules.Rules,
	blockChain *blockchain.BlockChain,
	opts *core.Options,
) (byz byzantine.Byzantine, ok bool) {
	ok = true
	switch name {
	case byzantine.SilenceModuleName:
		byz = byzantine.NewSilence(rules)
	case byzantine.ForkModuleName:
		byz = byzantine.NewFork(rules, blockChain, opts)
	default:
		return nil, false
	}
	return
}

func getLeaderRotation(
	name string,

	chainLength int,
	blockChain *blockchain.BlockChain,
	config *netconfig.Config,
	committer *committer.Committer,
	logger logging.Logger,
	opts *core.Options,
) (ld modules.LeaderRotation, ok bool) {
	ok = true
	switch name {
	case leaderrotation.CarouselModuleName:
		ld = leaderrotation.NewCarousel(chainLength, blockChain, config, committer, opts, logger)
	case leaderrotation.ReputationModuleName:
		ld = leaderrotation.NewRepBased(chainLength, config, committer, opts, logger)
	case leaderrotation.RoundRobinModuleName:
		ld = leaderrotation.NewRoundRobin(config)
	case leaderrotation.FixedModuleName:
		ld = leaderrotation.NewFixed(hotstuff.ID(1))
	case leaderrotation.TreeLeaderModuleName:
		ld = leaderrotation.NewTreeLeader(opts)
	default:
		return nil, false
	}
	return
}

func getCrypto(
	name string,
	configuration *netconfig.Config,
	logger logging.Logger,
	opts *core.Options,
) (crypto modules.CryptoBase, ok bool) {
	ok = true
	switch name {
	case bls12.ModuleName:
		crypto = bls12.New(configuration, logger, opts)
	case ecdsa.ModuleName:
		crypto = ecdsa.New(configuration, logger, opts)
	case eddsa.ModuleName:
		crypto = eddsa.New(configuration, logger, opts)
	default:
		return nil, false
	}
	return
}

type moduleList struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	eventLoop    *core.EventLoop
	synchronizer *synchronizer.Synchronizer
}

func setupModules(
	opts *orchestrationpb.ReplicaOpts,
	logger logging.Logger,
	privKey hotstuff.PrivateKey,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*moduleList, error) {
	moduleOpt := core.NewOptions(hotstuff.ID(opts.GetID()), privKey)
	moduleOpt.SetSharedRandomSeed(opts.GetSharedSeed())
	moduleOpt.SetTreeConfig(opts.GetBranchFactor(), opts.TreePositionIDs(), opts.TreeDeltaDuration())

	var duration modules.ViewDuration
	if opts.GetLeaderRotation() == "tree-leader" {
		duration = synchronizer.NewFixedViewDuration(opts.GetInitialTimeout().AsDuration())
	} else {
		duration = synchronizer.NewViewDuration(
			uint64(opts.GetTimeoutSamples()),
			float64(opts.GetInitialTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
			float64(opts.GetMaxTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
			float64(opts.GetTimeoutMultiplier()),
		)
	}
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
	netConfiguration := netconfig.NewConfig()
	cryptoImpl, ok := getCrypto(opts.GetCrypto(), netConfiguration, logger, moduleOpt)
	if !ok {
		return nil, fmt.Errorf("invalid crypto name: '%s'", opts.GetCrypto())
	}
	eventLoop := core.NewEventLoop(logger, 1000)
	sender := sender.New(
		netConfiguration,
		eventLoop,
		logger,
		moduleOpt,
		creds,
		gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()),
	)
	cmdCache := clientsrv.NewCmdCache(
		logger,
		int(opts.GetBatchSize()),
	)
	clientSrv := clientsrv.NewClientServer(
		eventLoop,
		logger,
		cmdCache,
		clientSrvOpts,
	)
	blockChain := blockchain.New(
		sender,
		eventLoop,
		logger,
	)
	server := server.NewServer(
		blockChain,
		netConfiguration,
		eventLoop,
		logger,
		moduleOpt,

		server.WithLatencies(hotstuff.ID(opts.GetID()), opts.GetLocations()),
		server.WithGorumsServerOptions(replicaSrvOpts...),
	)
	committer := committer.New(
		blockChain,
		clientSrv,
		logger,
	)
	certAuthority := certauth.NewCache(
		cryptoImpl,
		blockChain,
		netConfiguration,
		logger,
		100, // TODO: consider making this configurable
	)
	consensusRules, ok := getConsensusRules(opts.GetConsensus(), blockChain, logger, moduleOpt)
	if !ok {
		return nil, fmt.Errorf("invalid consensus name: '%s'", opts.GetConsensus())
	}
	leaderRotation, ok := getLeaderRotation(
		opts.GetLeaderRotation(),
		consensusRules.ChainLength(),
		blockChain,
		netConfiguration,
		committer,
		logger,
		moduleOpt,
	)
	if !ok {
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", opts.GetLeaderRotation())
	}
	var kauriModule *kauri.Kauri = nil
	if opts.GetKauri() {
		kauriModule = kauri.New(
			cryptoImpl,
			leaderRotation,
			blockChain,
			moduleOpt.TreeConfig(),
			moduleOpt,
			eventLoop,
			netConfiguration,
			sender,
			server,
			logger,
		)
	}
	strategy := opts.GetByzantineStrategy()
	if strategy != "" {
		if byz, ok := getByzantine(strategy, consensusRules, blockChain, moduleOpt); ok {
			consensusRules = byz.Wrap()
			logger.Infof("assigned byzantine strategy: %s", strategy)
		} else {
			return nil, fmt.Errorf("invalid byzantine strategy: '%s'", opts.GetByzantineStrategy())
		}
	}
	csus := consensus.New(
		consensusRules,
		leaderRotation,
		blockChain,
		committer,
		cmdCache,
		sender,
		kauriModule,
		certAuthority,
		eventLoop,
		logger,
		moduleOpt,
	)
	synch := synchronizer.New(
		cryptoImpl,
		leaderRotation,
		duration,
		blockChain,
		csus,
		certAuthority,
		netConfiguration,
		sender,
		eventLoop,
		logger,
		moduleOpt,
	)
	// No need to store votingMachine since it's not a dependency.
	// The constructor adds event handlers that enables voting logic.
	synchronizer.NewVotingMachine(
		blockChain,
		netConfiguration,
		certAuthority,
		eventLoop,
		logger,
		synch,
		moduleOpt,
	)
	return &moduleList{
		clientSrv:    clientSrv,
		server:       server,
		sender:       sender,
		eventLoop:    eventLoop,
		synchronizer: synch,
	}, nil
}
