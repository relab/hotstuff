package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

type protocolModules struct {
	consensusRules modules.ConsensusRules
	leaderRotation modules.LeaderRotation
}

// TODO: Add option for byzantineStrategy instead of passing string
func newProtocolModules(
	depsCore *Core,
	depsSecure *Security,
	depsSrv *Service,

	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdParams viewduration.Params,
) (*protocolModules, error) {
	consensusRules, err := newConsensusRulesModule(
		consensusName,
		depsSecure.BlockChain(),
		depsCore.Logger(),
		depsCore.Globals(),
	)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		leaderRotationName,
		consensusRules.ChainLength(),
		vdParams,
		depsSecure.BlockChain(),
		depsSrv.Committer(),
		depsCore.Logger(),
		depsCore.Globals(),
	)
	if err != nil {
		return nil, err
	}

	if byzantineStrategy != "" {
		byz, err := newByzantineStrategyModule(
			byzantineStrategy,
			consensusRules,
			depsSecure.BlockChain(),
			depsCore.Globals())
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		depsCore.Logger().Infof("assigned byzantine strategy: %s", byzantineStrategy)
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
		mods.consensusRules,
		mods.leaderRotation,
		depsSecure.BlockChain(),
		depsSrv.Committer(),
		depsSrv.CmdCache(),
		depsNet.Sender(),
		depsSecure.CertAuth(),
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.Globals(),
	)
	return &Protocol{
		consensus: csus,
		synchronizer: synchronizer.New(
			depsSecure.CryptoImpl(),
			mods.leaderRotation,
			depsSecure.BlockChain(),
			csus,
			depsSecure.CertAuth(),
			depsNet.Sender(),
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.Globals(),
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
