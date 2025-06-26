package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

type Consensus struct {
	voter     *consensus.Voter
	proposer  *consensus.Proposer
	committer *consensus.Committer
}

func NewConsensus(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	commandCache *clientpb.CommandCache,
	consensusRules consensus.Ruleset,
	leaderRotation leaderrotation.LeaderRotation,
	viewStates *protocol.ViewStates,
	comm comm.Communication,
) *Consensus {
	committer := consensus.NewCommitter(
		eventLoop,
		logger,
		blockchain,
		viewStates,
		consensusRules,
	)
	voter := consensus.NewVoter(
		config,
		leaderRotation,
		consensusRules,
		comm,
		auth,
		committer,
	)
	return &Consensus{
		committer: committer,
		voter:     voter,
		proposer: consensus.NewProposer(
			eventLoop,
			config,
			blockchain,
			viewStates,
			consensusRules,
			comm,
			voter,
			commandCache,
			committer,
		),
	}
}

// Proposer returns the proposer instance.
func (p *Consensus) Proposer() *consensus.Proposer {
	return p.proposer
}

// Voter returns the voter instance.
func (p *Consensus) Voter() *consensus.Voter {
	return p.voter
}

// Committer returns the committer instance.
func (p *Consensus) Committer() *consensus.Committer {
	return p.committer
}
