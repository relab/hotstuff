package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

type HotStuff struct {
	logger         logging.Logger
	config         *core.RuntimeConfig
	votingMachine  *VotingMachine
	leaderRotation modules.LeaderRotation
	sender         modules.Sender
}

func NewHotStuff(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	auth *cert.Authority,
	states *ViewStates,
	leaderRotation modules.LeaderRotation,
	sender modules.Sender,
) modules.ConsensusProtocol {
	return &HotStuff{
		logger: logger,
		config: config,
		votingMachine: NewVotingMachine(
			logger,
			eventLoop,
			config,
			blockChain,
			auth,
			states,
		),
		leaderRotation: leaderRotation,
		sender:         sender,
	}
}

func (cs *HotStuff) SendPropose(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	cs.sender.Propose(proposal)
}

// SendVote disseminates or stores a valid vote depending on replica being voter or leader in the next view.
func (cs *HotStuff) SendVote(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	leaderID := cs.leaderRotation.GetLeader(proposal.Block.View() + 1)
	if leaderID == cs.config.ID() {
		// if I am the leader in the next view, collect the vote for myself beforehand.
		cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
		return
	}
	// if I am the one voting, send the vote to next leader over the wire.
	err := cs.sender.Vote(leaderID, pc)
	if err != nil {
		cs.logger.Warnf("%v", err)
		return
	}
	cs.logger.Debugf("voting for %v", proposal)
}

var _ modules.ConsensusProtocol = (*HotStuff)(nil)
