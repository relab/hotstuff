package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
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
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	commandCache *clientpb.CommandCache,
	committer *consensus.Committer,
	consensusRules modules.HotstuffRuleset,
	leaderRotation modules.LeaderRotation,
	disAgg modules.DisseminatorAggregator,
) *Consensus {
	voter := consensus.NewVoter(
		config,
		leaderRotation,
		consensusRules,
		disAgg,
		auth,
		committer,
	)
	return &Consensus{
		voter: voter,
		proposer: consensus.NewProposer(
			eventLoop,
			config,
			blockchain,
			consensusRules,
			disAgg,
			voter,
			commandCache,
			committer,
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
