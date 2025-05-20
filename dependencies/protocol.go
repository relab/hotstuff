package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/viewstates"
)

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
	consensusRulesModule modules.ConsensusRules,
	leaderRotationModule modules.LeaderRotation,
	consensusOpts ...consensus.Option,
) (*Protocol, error) {
	ruler, ok := consensusRulesModule.(modules.ProposeRuler)
	if ok {
		consensusOpts = append(consensusOpts, consensus.OverrideProposeRule(ruler))
	}
	state := viewstates.New(
		depsCore.Logger(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	vm := consensus.NewVotingMachine(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
		state,
	)
	voter := consensus.NewVoter(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		leaderRotationModule,
		consensusRulesModule,
		depsSecure.BlockChain(),
		depsSecure.CertAuth(),
	)
	csus := consensus.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		leaderRotationModule,
		consensusRulesModule,
		voter,
		vm,
		depsSrv.Committer(),
		depsSrv.CmdCache(),
		depsNet.Sender(),
		consensusOpts...,
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
