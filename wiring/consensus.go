package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/service/committer"
)

type Consensus struct {
	voter    *consensus.Voter
	proposer *consensus.Proposer
}

func NewConsensus(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	auth *cert.Authority,
	commandCache *clientpb.Cache,
	committer *committer.Committer,
	consensusRulesModule modules.HotstuffRuleset,
	leaderRotationModule modules.LeaderRotation,
	protocol modules.ConsensusProtocol,
) *Consensus {
	proposerOpts := []consensus.ProposerOption{}
	if ruler, ok := consensusRulesModule.(modules.ProposeRuler); ok {
		proposerOpts = append(proposerOpts, consensus.OverrideProposeRule(ruler))
	}
	voter := consensus.NewVoter(
		eventLoop,
		logger,
		config,
		leaderRotationModule,
		consensusRulesModule,
		protocol,
		auth,
		commandCache,
		committer,
	)
	return &Consensus{
		voter: voter,
		proposer: consensus.NewProposer(
			eventLoop,
			logger,
			config,
			protocol,
			voter,
			commandCache,
			committer,
			proposerOpts...,
		),
	}
}

// Consensus returns the consensus protocol instance.
func (p *Consensus) Proposer() *consensus.Proposer {
	return p.proposer
}

// Synchronizer returns the synchronizer instance.
func (p *Consensus) Voter() *consensus.Voter {
	return p.voter
}
