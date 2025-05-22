package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/hsproposehandler"
	"github.com/relab/hotstuff/protocol/kauri"
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
		depsSecure.CertAuth(),
	)
	proposerOpts := []proposer.Option{}
	if ruler, ok := consensusRulesModule.(modules.ProposeRuler); ok {
		proposerOpts = append(proposerOpts, proposer.OverrideProposeRule(ruler))
	}
	proposer := proposer.New(
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSrv.cmdCache,
		proposerOpts...,
	)
	votingMachine := votingmachine.New(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
		state,
	)
	var handler modules.ExtProposeHandler
	// TODO(AlanRostem): add module logic for this.
	if depsCore.RuntimeCfg().KauriEnabled() {
		handler = kauri.New(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.CertAuth(),
			sender.(*network.GorumsSender), // TODO(AlanRostem): avoid cast
		)
	} else {
		handler = hsproposehandler.New(
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			voter,
			votingMachine,
			leaderRotationModule,
			sender,
		)
	}
	csus := consensus.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		handler,
		proposer,
		voter,
		votingMachine,
		depsSrv.Committer(),
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
