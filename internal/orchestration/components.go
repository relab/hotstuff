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
) (rules modules.ConsensusRules, ok bool) {
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
	rules modules.ConsensusRules,
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
	cachedCertAuth bool,
) (*SecurityComponents, error) {
	blockChain := blockchain.New(
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
	)
	cryptoImpl, ok := getCrypto(cryptoName, netComps.Config, coreComps.Logger, coreComps.Options)
	if !ok {
		return nil, fmt.Errorf("invalid crypto name: '%s'", cryptoName)
	}
	var certAuthority *certauth.CertAuthority
	if cachedCertAuth {
		certAuthority = certauth.NewCached(
			cryptoImpl,
			blockChain,
			netComps.Config,
			coreComps.Logger,
			100, // TODO: consider making this configurable
		)
	} else {
		certAuthority = certauth.New(
			cryptoImpl,
			blockChain,
			netComps.Config,
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
	consensusName, leaderRotationName, byzantineName string,
	vdOpt ViewDurationOptions,
) (*ProtocolModules, error) {
	consensusRules, ok := getConsensusRules(consensusName, secureComps.BlockChain, coreComps.Logger, coreComps.Options)
	if !ok {
		return nil, fmt.Errorf("invalid consensus name: '%s'", consensusName)
	}
	leaderRotation, ok := getLeaderRotation(
		leaderRotationName,
		consensusRules.ChainLength(),
		secureComps.BlockChain,
		netComps.Config,
		srvComps.Committer,
		coreComps.Logger,
		coreComps.Options,
	)
	if !ok {
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", leaderRotationName)
	}
	var duration modules.ViewDuration
	if leaderRotationName == "tree-leader" {
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
	if byzantineName != "" {
		if byz, ok := getByzantine(byzantineName, consensusRules, secureComps.BlockChain, coreComps.Options); ok {
			consensusRules = byz.Wrap()
			coreComps.Logger.Infof("assigned byzantine strategy: %s", byzantineName)
		} else {
			return nil, fmt.Errorf("invalid byzantine strategy: '%s'", byzantineName)
		}
	}
	return &ProtocolModules{
		ConsensusRules: consensusRules,
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
		secureComps.BlockChain,
		srvComps.Committer,
		srvComps.CmdCache,
		netComps.Sender,
		secureComps.CertAuth,
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
	// No need to store votingMachine since it's not a dependency.
	// The constructor adds event handlers that enables voting logic.
	// The registered event handlers also prevent this object from being disposed.
	synchronizer.NewVotingMachine(
		secureComps.BlockChain,
		netComps.Config,
		secureComps.CertAuth,
		coreComps.EventLoop,
		coreComps.Logger,
		synch,
		coreComps.Options,
	)
	return &ProtocolComponents{
		Consensus:    csus,
		Synchronizer: synch,
	}
}

type replicaComponents struct {
	clientSrv    *clientsrv.ClientServer
	server       *server.Server
	sender       *sender.Sender
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
}

func newReplicaComponents(
	opts *orchestrationpb.ReplicaOpts,
	privKey hotstuff.PrivateKey,
	certificate tls.Certificate,
	rootCAs *x509.CertPool,
) (*replicaComponents, error) {

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
		gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()),
	)
	secureComps, err := NewSecurityComponents(coreComps, netComps, opts.GetCrypto(), true)
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

	// Putting it here for now since it breaks consistency with component categorization.
	// TODO(AlanRostem): figure out a better way to enable Kauri. Problem is that it needs server, making categorization harder.
	if opts.GetKauri() {
		protocolComps.Consensus.SetKauri(kauri.New(
			secureComps.CryptoImpl,
			protocolMods.LeaderRotation,
			secureComps.BlockChain,
			coreComps.Options,
			coreComps.EventLoop,
			netComps.Config,
			netComps.Sender,
			server,
			coreComps.Logger,
		))
	}
	return &replicaComponents{
		clientSrv:    srvComps.ClientSrv,
		server:       server,
		sender:       netComps.Sender,
		synchronizer: protocolComps.Synchronizer,
		eventLoop:    coreComps.EventLoop,
		logger:       coreComps.Logger,
	}, nil
}
