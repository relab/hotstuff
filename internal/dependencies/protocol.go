package dependencies

import (
	"time"

	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/synchronizer"
)

type protocolModules struct {
	ConsensusRules modules.ConsensusRules
	Kauri          modules.Kauri
	LeaderRotation modules.LeaderRotation
	ViewDuration   modules.ViewDuration
}

type ViewDurationOptions struct {
	IsFixed      bool
	SampleSize   uint64
	StartTimeout float64
	MaxTimeout   float64
	Multiplier   float64
}

func newProtocolModules(
	depsCore *Core,
	depsNet *Network,
	depsSecure *Security,
	depsSrv *Service,

	useKauri bool,
	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdOpt ViewDurationOptions,
) (*protocolModules, error) {
	consensusRules, err := newConsensusRulesModule(consensusName, depsSecure.BlockChain, depsCore.Logger, depsCore.Options)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		leaderRotationName,
		consensusRules.ChainLength(),
		depsSecure.BlockChain,
		depsNet.Config,
		depsSrv.Committer,
		depsCore.Logger,
		depsCore.Options,
	)
	if err != nil {
		return nil, err
	}

	var kauriOptional modules.Kauri = nil

	if useKauri {
		kauriOptional = kauri.New(
			depsSecure.CryptoImpl,
			leaderRotation,
			depsSecure.BlockChain,
			depsCore.Options,
			depsCore.EventLoop,
			depsNet.Config,
			depsNet.Sender,
			depsCore.Logger,
		)
	}

	var duration modules.ViewDuration
	if vdOpt.IsFixed {
		duration = synchronizer.NewFixedViewDuration(time.Duration(vdOpt.StartTimeout * float64(time.Millisecond)))
	} else {
		duration = synchronizer.NewViewDuration(
			vdOpt.SampleSize, vdOpt.StartTimeout, vdOpt.MaxTimeout, vdOpt.Multiplier,
		)
	}
	if byzantineStrategy != "" {
		byz, err := newByzantineStrategyModule(
			byzantineStrategy,
			consensusRules,
			depsSecure.BlockChain,
			depsCore.Options)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		depsCore.Logger.Infof("assigned byzantine strategy: %s", byzantineStrategy)
	}
	return &protocolModules{
		ConsensusRules: consensusRules,
		Kauri:          kauriOptional,
		LeaderRotation: leaderRotation,
		ViewDuration:   duration,
	}, nil
}

type Protocol struct {
	Consensus    *consensus.Consensus
	Synchronizer *synchronizer.Synchronizer
}

func NewProtocol(
	depsCore *Core,
	depsNet *Network,
	depsSecure *Security,
	depsSrv *Service,

	useKauri bool,
	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdOpt ViewDurationOptions,
) (*Protocol, error) {
	mods, err := newProtocolModules(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		useKauri,
		consensusName,
		leaderRotationName,
		byzantineStrategy,
		vdOpt,
	)
	if err != nil {
		return nil, err
	}
	csus := consensus.New(
		mods.ConsensusRules,
		mods.LeaderRotation,
		mods.Kauri,
		depsSecure.BlockChain,
		depsSrv.Committer,
		depsSrv.CmdCache,
		depsNet.Sender,
		depsSecure.CertAuth,
		depsNet.Config,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
	)
	synch := synchronizer.New(
		depsSecure.CryptoImpl,
		mods.LeaderRotation,
		mods.ViewDuration,
		depsSecure.BlockChain,
		csus,
		depsSecure.CertAuth,
		depsNet.Config,
		depsNet.Sender,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
	)
	return &Protocol{
		Consensus:    csus,
		Synchronizer: synch,
	}, nil
}
