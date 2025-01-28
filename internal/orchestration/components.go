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
	"github.com/relab/hotstuff/eventloop"
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
	opts := core.NewOptions(id, privKey)
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	eventLoop := eventloop.New(logger, 100)
	comps := &CoreComponents{
		Options:   opts,
		Logger:    logger,
		EventLoop: eventLoop,
	}
	return comps
}

type NetworkedComponents struct {
	Config *netconfig.Config
	Sender *sender.Sender
}

func NewNetworkedComponents(
	coreComps *CoreComponents,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *NetworkedComponents {
	cfg := netconfig.NewConfig()
	send := sender.New(
		cfg,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
		creds,
		mgrOpts...,
	)

	comps := &NetworkedComponents{
		Config: cfg,
		Sender: send,
	}
	return comps
}

type SecureComponents struct {
	BlockChain *blockchain.BlockChain
	CryptoImpl modules.CryptoBase
	CertAuth   *certauth.CertAuthority
}

func NewSecureComponents(
	coreComps *CoreComponents,
	netComps *NetworkedComponents,
	cryptoName string,

) (*SecureComponents, error) {
	blockChain := blockchain.New(
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
	)
	cryptoImpl, ok := getCrypto(cryptoName, netComps.Config, coreComps.Logger, coreComps.Options)
	if !ok {
		return nil, fmt.Errorf("invalid crypto name: '%s'", cryptoName)
	}
	certAuthority := certauth.NewCache(
		cryptoImpl,
		blockChain,
		netComps.Config,
		coreComps.Logger,
		100, // TODO: consider making this configurable
	)
	comps := &SecureComponents{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth:   certAuthority,
	}
	return comps, nil
}

type ServiceComponents struct {
	CmdCache  *clientsrv.CmdCache
	Server    *server.Server
	ClientSrv *clientsrv.ClientServer
	Committer *committer.Committer
}

func NewServiceComponents(
	coreComps *CoreComponents,
	netComps *NetworkedComponents,
	secureComps *SecureComponents,
	id hotstuff.ID,
	batchSize int,
	locations []string,
	clientSrvOpts []gorums.ServerOption,
	replicaSrvOpts []gorums.ServerOption,
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
	server := server.NewServer(
		secureComps.BlockChain,
		netComps.Config,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,

		server.WithLatencies(id, locations),
		server.WithGorumsServerOptions(replicaSrvOpts...),
	)
	comps := &ServiceComponents{
		CmdCache:  cmdCache,
		ClientSrv: clientSrv,
		Committer: committer,
		Server:    server,
	}
	return comps
}

type compList struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	coreComps    *CoreComponents
}

func setupComps(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*compList, error) {

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

	coreComps := NewCoreComponents(opts.HotstuffID(), "hs", privKey)
	coreComps.Options.SetSharedRandomSeed(opts.GetSharedSeed())
	coreComps.Options.SetTreeConfig(opts.GetBranchFactor(), opts.TreePositionIDs(), opts.TreeDeltaDuration())

	netComps := NewNetworkedComponents(
		coreComps,
		creds,
		gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()))
	secureComps, err := NewSecureComponents(coreComps, netComps, opts.GetCrypto())
	if err != nil {
		return nil, err
	}
	srvComps := NewServiceComponents(
		coreComps, netComps, secureComps,
		opts.HotstuffID(), int(opts.GetBatchSize()), opts.GetLocations(),
		clientSrvOpts, replicaSrvOpts,
	)
	consensusRules, ok := getConsensusRules(opts.GetConsensus(), secureComps.BlockChain, coreComps.Logger, coreComps.Options)
	if !ok {
		return nil, fmt.Errorf("invalid consensus name: '%s'", opts.GetConsensus())
	}
	leaderRotation, ok := getLeaderRotation(
		opts.GetLeaderRotation(),
		consensusRules.ChainLength(),
		secureComps.BlockChain,
		netComps.Config,
		srvComps.Committer,
		coreComps.Logger,
		coreComps.Options,
	)
	if !ok {
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", opts.GetLeaderRotation())
	}
	var kauriModule *kauri.Kauri = nil
	if opts.GetKauri() {
		kauriModule = kauri.New(
			secureComps.CryptoImpl,
			leaderRotation,
			secureComps.BlockChain,
			coreComps.Options.TreeConfig(),
			coreComps.Options,
			coreComps.EventLoop,
			netComps.Config,
			netComps.Sender,
			srvComps.Server,
			coreComps.Logger,
		)
	}
	strategy := opts.GetByzantineStrategy()
	if strategy != "" {
		if byz, ok := getByzantine(strategy, consensusRules, secureComps.BlockChain, coreComps.Options); ok {
			consensusRules = byz.Wrap()
			coreComps.Logger.Infof("assigned byzantine strategy: %s", strategy)
		} else {
			return nil, fmt.Errorf("invalid byzantine strategy: '%s'", opts.GetByzantineStrategy())
		}
	}
	csus := consensus.New(
		consensusRules,
		leaderRotation,
		secureComps.BlockChain,
		srvComps.Committer,
		srvComps.CmdCache,
		netComps.Sender,
		kauriModule,
		secureComps.CertAuth,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
	)
	synch := synchronizer.New(
		secureComps.CryptoImpl,
		leaderRotation,
		duration,
		secureComps.BlockChain,
		csus,
		secureComps.CertAuth,
		netComps.Config,
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
		coreComps.Options,
	)
	// No need to store votingMachine since it's not a dependency.
	// The constructor adds event handlers that enables voting logic.
	synchronizer.NewVotingMachine(
		secureComps.BlockChain,
		netComps.Config,
		secureComps.CertAuth,
		coreComps.EventLoop,
		coreComps.Logger,
		synch,
		coreComps.Options,
	)
	return &compList{
		clientSrv:    srvComps.ClientSrv,
		server:       srvComps.Server,
		sender:       netComps.Sender,
		synchronizer: synch,
		coreComps:    coreComps,
	}, nil
}
