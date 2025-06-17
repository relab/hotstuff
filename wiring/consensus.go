package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

type Consensus struct {
	voter    *consensus.Voter
	proposer *consensus.Proposer
}

func NewConsensus(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	auth *cert.Authority,
	commandCache *clientpb.CommandCache,
	committer *committer.Committer,
	consensusRulesModule modules.HotstuffRuleset,
	leaderRotationModule modules.LeaderRotation,
	protocol modules.DisseminatorAggregator,
) *Consensus {
	proposerOpts := []consensus.ProposerOption{}
	if ruler, ok := consensusRulesModule.(modules.ProposeRuler); ok {
		proposerOpts = append(proposerOpts, consensus.OverrideProposeRule(ruler))
	}
	voter := consensus.NewVoter(
		logger,
		config,
		leaderRotationModule,
		consensusRulesModule,
		protocol,
		auth,
		committer,
	)
	return &Consensus{
		voter: voter,
		proposer: consensus.NewProposer(
			eventLoop,
			logger,
			config,

			blockChain,
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
