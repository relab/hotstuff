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
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	})
	return cs
}

// OnPropose is called when receiving a proposal from a leader and returns true if the proposal was voted for.
func (cs *Consensus) OnPropose(proposal hotstuff.ProposeMsg) {
	block := proposal.Block
	// ensure that I can vote in this view based on the protocol's rule.
	if !cs.voter.Verify(&proposal) {
		return
	}
	// if we can't commit the block, don't vote for it.
	if !cs.committer.TryCommit(block) {
		return
	}
	// even if voting fails, we should be able to go to the next view.
	newInfo := hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       cs.config.ID(),
		SyncInfo: newInfo,
	})
	// try to vote for the block and retrieve its partial certificate.
	pc, ok := cs.voter.Vote(block)
	// don't send the vote if it failed
	if !ok {
		return
	}
	// TODO(AlanRostem): make the below if-else block into an interface
	// kauri will handle sending over the wire differently, so we return here.
	if cs.kauri != nil {
		// disseminate proposal and aggregate votes.
		cs.kauri.Begin(pc, proposal)
	} else {
		// if the proposal was voted for (state is updated internally), we send it over the wire.
		cs.sendVote(proposal, pc)
	}
	return
}

// Propose creates a new outgoing proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) {
	proposal, ok := cs.proposer.TryPropose(view, highQC, syncInfo)
	if !ok {
		return
	}
	block := proposal.Block
	// as proposer, I can vote for my own proposal without verifying.
	// NOTE: this vote call is not likely to fail since the leader does it.
	pc, ok := cs.voter.Vote(block)
	if !ok {
		cs.logger.Warnf("voteSelf[v=%d]: could not vote for my own proposal.", block.View())
		return
	}
	// can collect my own vote as leader
	cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	// TODO(AlanRostem): make the below if-else block into an interface
	// kauri will handle sending over the wire differently.
	if cs.kauri != nil {
		// disseminate proposal and aggregate votes.
		cs.kauri.Begin(pc, proposal)
	} else {
		// kauri sends the proposal to only the children
		cs.sender.Propose(proposal)
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

type ProposeHandler interface {
	HandlePropose(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert)
}

type ProposeDisseminator interface {
	DisseminatePropose(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert)
}
