package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

type protocolModules struct {
	ConsensusRules modules.ConsensusRules
	Kauri          modules.Kauri
	LeaderRotation modules.LeaderRotation
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
	vdOpt viewduration.Options,
) (*protocolModules, error) {
	consensusRules, err := newConsensusRulesModule(consensusName, depsSecure.BlockChain, depsCore.Logger, depsCore.Options)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		leaderRotationName,
		consensusRules.ChainLength(),
		vdOpt,
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
	vdOpt viewduration.Options,
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
