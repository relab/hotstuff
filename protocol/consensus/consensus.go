package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/proposer"
	"github.com/relab/hotstuff/protocol/voter"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/committer"
)

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	committer    *committer.Committer
	commandCache *cmdcache.Cache

	impl           modules.ConsensusRules
	leaderRotation modules.LeaderRotation

	voter         *voter.Voter
	votingMachine *votingmachine.VotingMachine
	proposer      *proposer.Proposer

	kauri *kauri.Kauri

	sender modules.Sender
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(
	// core dependencies
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	// security dependencies
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,

	// protocol dependencies
	leaderRotation modules.LeaderRotation,
	impl modules.ConsensusRules,
	proposer *proposer.Proposer,
	voter *voter.Voter,
	votingMachine *votingmachine.VotingMachine,

	// service dependencies
	committer *committer.Committer,
	commandCache *cmdcache.Cache,

	// network dependencies
	sender *network.GorumsSender,
) *Consensus {
	cs := &Consensus{
		impl:           impl,
		leaderRotation: leaderRotation,
		kauri:          nil, // this is set later based on config

		committer:    committer,
		commandCache: commandCache,
		eventLoop:    eventLoop,
		logger:       logger,
		config:       config,
		sender:       sender,
		proposer:     proposer,

		voter:         voter,
		votingMachine: votingMachine,
	}
	if config.KauriEnabled() {
		cs.kauri = kauri.New(
			logger,
			eventLoop,
			config,
			blockChain,
			auth,
			sender,
		)
	}
	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.logger.Debugf("On event (hotstuff.ProposeMsg).")
		proposal := event.(hotstuff.ProposeMsg)
		if !cs.OnPropose(proposal) {
			// if it failed to process the proposal, don't advance the view
			return
		}
		newInfo := hotstuff.NewSyncInfo().WithQC(proposal.Block.QuorumCert())
		cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
			ID:       cs.config.ID(),
			SyncInfo: newInfo,
		})
	})
	return cs
}

// OnPropose is called when receiving a proposal from a leader and returns true if the proposal was voted for.
func (cs *Consensus) OnPropose(proposal hotstuff.ProposeMsg) (advance bool) {
	block := proposal.Block
	advance = false // won't increment the view when the below if-statements return
	// ensure that I can vote in this view based on the protocol's rule.
	if !cs.voter.Verify(&proposal) {
		return
	}
	// kauri will handle sending over the wire differently, so we return here.
	// try to vote for the block and retrieve its partial certificate.
	pc, advance := cs.voter.Vote(block)
	if cs.kauri != nil {
		// disseminate proposal and aggregate votes.
		cs.kauri.Begin(pc, proposal)
		return
	}
	// TODO(AlanRostem): before this commit, advance was set to true despite the vote failing.
	// Not sure if that was correct behavior.
	// advance tells the synchronizer to advance the view
	if !advance {
		return
	}
	// if the proposal was voted for (state is updated internally), we send it over the wire.
	cs.sendVote(proposal, pc)
	return
}

// Propose creates a new outgoing proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) {
	proposal, ok := cs.proposer.TryPropose(view, highQC, syncInfo)
	if !ok {
		return
	}
	// kauri sends the proposal to only the children
	if cs.kauri == nil {
		cs.sender.Propose(proposal)
	}
	block := proposal.Block
	// as proposer, I can vote for my own proposal without verifying
	pc, ok := cs.voter.Vote(block)
	if !ok {
		cs.logger.Warnf("voteSelf[v=%d]: could not vote for my own proposal.", block.View())
		return
	}
	// can collect my own vote.
	cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	// kauri will handle sending over the wire differently.
	if cs.kauri != nil {
		// disseminate proposal and aggregate votes.
		cs.kauri.Begin(pc, proposal)
	}
}

func (cs *Consensus) sendVote(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	block := proposal.Block
	view := block.View()
	leaderID := cs.leaderRotation.GetLeader(cs.voter.LastVote() + 1)
	if leaderID == cs.config.ID() {
		// if I am the leader in the next view, collect the vote for myself beforehand.
		cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
		return
	}
	// if I am the one voting, sent the vote to next leader over the wire.
	err := cs.sender.Vote(leaderID, pc)
	if err != nil {
		cs.logger.Warnf("%v", err)
		return
	}
	cs.logger.Debugf("TryVote[view=%d]: voting for %v", view, block)
}
