package dependencies

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/proposer"
	"github.com/relab/hotstuff/protocol/voter"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/cmdcache"
)

type Consensus struct {
	voter    *voter.Voter
	proposer *proposer.Proposer
}

func NewConsensus(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	auth *certauth.CertAuthority,
	commandCache *cmdcache.Cache,
	consensusRulesModule modules.HotstuffRuleset,
	leaderRotationModule modules.LeaderRotation,
) *Consensus {
	proposerOpts := []proposer.Option{}
	if ruler, ok := consensusRulesModule.(modules.ProposeRuler); ok {
		proposerOpts = append(proposerOpts, proposer.OverrideProposeRule(ruler))
	}
	return &Consensus{
		voter: voter.New(
			eventLoop,
			logger,
			config,
			leaderRotationModule,
			consensusRulesModule,
			auth,
			commandCache,
		),
		proposer: proposer.New(
			eventLoop,
			config,
			commandCache,
			proposerOpts...,
		),
	}
}

// Consensus returns the consensus protocol instance.
func (p *Consensus) Proposer() *proposer.Proposer {
	return p.proposer
}

// Synchronizer returns the synchronizer instance.
func (p *Consensus) Voter() *voter.Voter {
	return p.voter
}
