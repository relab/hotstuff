package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/proposer"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/viewstates"
	"github.com/relab/hotstuff/protocol/voter"
	"github.com/relab/hotstuff/protocol/votingmachine"
)

type Protocol struct {
	consensus    *consensus.Consensus
	synchronizer *synchronizer.Synchronizer
}

// TODO(AlanRostem): explore ways to simplify consensus and synchronizer so that this method takes in less dependencies.
func NewProtocol(
	depsCore *Core,
	depsSecure *Security,
	depsSrv *Service,
	sender modules.Sender,
	consensusRulesModule modules.ConsensusRules,
	leaderRotationModule modules.LeaderRotation,
) (*Protocol, error) {
	opts := []proposer.Option{}
	if ruler, ok := consensusRulesModule.(modules.ProposeRuler); ok {
		opts = append(opts, proposer.OverrideProposeRule(ruler))
	}
	state := viewstates.New(
		depsCore.Logger(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	voter := voter.New(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		leaderRotationModule,
		consensusRulesModule,
		depsSrv.Committer(),
		depsSecure.CertAuth(),
	)
	proposer := proposer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSrv.cmdCache,
		opts...,
	)
	votingMachine := votingmachine.New(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
		state,
	)
	csus := consensus.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
		leaderRotationModule,
		consensusRulesModule,
		proposer,
		voter,
		votingMachine,
		depsSrv.Committer(),
		depsSrv.CmdCache(),
		sender.(*network.GorumsSender), // TODO(AlanRostem): remove this after decoupling kauri
	)
	return &Protocol{
		consensus: csus,
		synchronizer: synchronizer.New(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecure.CertAuth(),
			leaderRotationModule,
			csus,
			voter,
			state,
			sender,
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
