package dependencies

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/committer"
)

type protocolModules struct {
	consensusRules modules.ConsensusRules
	leaderRotation modules.LeaderRotation
}

func newProtocolModules(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	committer *committer.Committer,

	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdParams viewduration.Params,
) (*protocolModules, error) {
	consensusRules, err := newConsensusRulesModule(
		logger,
		config,
		blockChain,
		consensusName,
	)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		logger,
		config,
		blockChain,
		committer,
		vdParams,
		leaderRotationName,
		consensusRules.ChainLength(),
	)
	if err != nil {
		return nil, err
	}
	if byzantineStrategy != "" {
		byz, err := newByzantineStrategyModule(
			config,
			blockChain,
			consensusRules,
			byzantineStrategy,
		)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		logger.Infof("assigned byzantine strategy: %s", byzantineStrategy)
	}
	return &protocolModules{
		consensusRules: consensusRules,
		leaderRotation: leaderRotation,
	}, nil
}

type Protocol struct {
	consensus    *consensus.Consensus
	synchronizer *synchronizer.Synchronizer
}

// TODO(AlanRostem): explore ways to simplify consensus and synchronizer so that this method takes in less dependencies.
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
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSrv.Committer(),
		consensusName,
		leaderRotationName,
		byzantineStrategy,
		vdParams,
	)
	if err != nil {
		return nil, err
	}
	opts := []consensus.Option{}
	if ruler, ok := mods.consensusRules.(modules.ProposeRuler); ok {
		opts = append(opts, consensus.OverrideProposeRule(ruler))
	}
	voter := consensus.NewVoter(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		mods.leaderRotation,
		mods.consensusRules,
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	leader := consensus.NewLeader(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	csus := consensus.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
		mods.leaderRotation,
		mods.consensusRules,
		voter,
		leader,
		depsSrv.Committer(),
		depsSrv.CmdCache(),
		depsNet.Sender(),
		opts...,
	)
	return &Protocol{
		consensus: csus,
		synchronizer: synchronizer.New(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecure.CertAuth(),
			mods.leaderRotation,
			csus,
			voter,
			depsNet.Sender(),
		),
	}, nil
}

// Consensus returns the consensus protocol instance.
func (p *Protocol) Consensus() *consensus.Consensus {
	return p.consensus
}

// Synchronizer returns the synchronizer instance.
func (p *Protocol) Synchronizer() *synchronizer.Synchronizer {
	return p.synchronizer
}
