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
	states         *ViewStates
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
		states:         states,
		leaderRotation: leaderRotation,
		sender:         sender,
	}
}

func (hs *HotStuff) SendPropose(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	hs.votingMachine.CollectVote(hotstuff.VoteMsg{
		ID:          hs.config.ID(),
		PartialCert: pc,
	})
	hs.sender.Propose(proposal)
}

// SendVote disseminates or stores a valid vote depending on replica being voter or leader in the next view.
func (hs *HotStuff) SendVote(lastVote hotstuff.View, proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	leaderID := hs.leaderRotation.GetLeader(lastVote + 1)
	if leaderID == hs.config.ID() {
		// if I am the leader in the next view, collect the vote for myself beforehand.
		hs.votingMachine.CollectVote(hotstuff.VoteMsg{
			ID:          hs.config.ID(),
			PartialCert: pc,
		})
		return
	}
	// if I am the one voting, send the vote to next leader over the wire.
	if err := hs.sender.Vote(leaderID, pc); err != nil {
		hs.logger.Warnf("%v", err)
		return
	}
}

var _ modules.ConsensusProtocol = (*HotStuff)(nil)
