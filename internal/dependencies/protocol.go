package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
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

	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdParams viewduration.Params,
) (*protocolModules, error) {
	consensusRules, err := newConsensusRulesModule(consensusName, depsSecure.BlockChain, depsCore.Logger, depsCore.Globals)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		leaderRotationName,
		consensusRules.ChainLength(),
		vdParams,
		depsSecure.BlockChain,
		depsNet.Config,
		depsSrv.Committer,
		depsCore.Logger,
		depsCore.Globals,
	)
	if err != nil {
		return nil, err
	}

	if byzantineStrategy != "" {
		byz, err := newByzantineStrategyModule(
			byzantineStrategy,
			consensusRules,
			depsSecure.BlockChain,
			depsCore.Globals)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		depsCore.Logger.Infof("assigned byzantine strategy: %s", byzantineStrategy)
	}
	return &protocolModules{
		ConsensusRules: consensusRules,
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

	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdParams viewduration.Params,
) (*Protocol, error) {
	mods, err := newProtocolModules(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		consensusName,
		leaderRotationName,
		byzantineStrategy,
		vdParams,
	)
	if err != nil {
		return nil, err
	}
	csus := consensus.New(
		mods.ConsensusRules,
		mods.LeaderRotation,
		depsSecure.BlockChain,
		depsSrv.Committer,
		depsSrv.CmdCache,
		depsNet.Sender,
		depsSecure.CertAuth,
		depsNet.Config,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Globals,
	)
	return &Protocol{
		Consensus: csus,
		Synchronizer: synchronizer.New(
			depsSecure.CryptoImpl,
			mods.LeaderRotation,
			depsSecure.BlockChain,
			csus,
			depsSecure.CertAuth,
			depsNet.Config,
			depsNet.Sender,
			depsCore.EventLoop,
			depsCore.Logger,
			depsCore.Globals,
		),
	}, nil
}
