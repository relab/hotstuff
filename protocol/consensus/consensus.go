package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
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
	ruler          modules.ProposeRuler
	leaderRotation modules.LeaderRotation
	voter          *voter.Voter
	votingMachine  *votingmachine.VotingMachine
	kauri          *kauri.Kauri

	sender *network.Sender
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
	voter *voter.Voter,
	votingMachine *votingmachine.VotingMachine,

	// service dependencies
	committer *committer.Committer,
	commandCache *cmdcache.Cache,

	// network dependencies
	sender *network.Sender,

	// options
	opts ...Option,
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

		voter:         voter,
		votingMachine: votingMachine,
	}
	cs.ruler = cs
	for _, opt := range opts {
		opt(cs)
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

// OnPropose is called when receiving a proposal from a leader.
func (cs *Consensus) OnPropose(proposal hotstuff.ProposeMsg) (advance bool) {
	block := proposal.Block
	advance = false // won't increment the view when the below if-statements return
	// ensure that I can vote in this view based on the protocol's rule.
	if !cs.voter.TryAccept(&proposal) {
		return
	}
	// now the proposal is valid, so we can increment the view, but the command may
	// still be too old to execute
	if !cs.committer.TryCommit(proposal.Block) {
		return
	}
	advance = true // Tells the synchronizer to advance the view, even if vote creation failed.
	// try to vote for the block and retrieve its partial certificate.
	pc, ok := cs.voter.Vote(block)
	if !ok {
		return
	}
	// kauri will handle sending over the wire differently, so we return here.
	if cs.kauri != nil {
		// disseminate proposal and aggregate votes.
		cs.kauri.Begin(pc, proposal)
		return
	}
	// if the proposal was voted for (state is updated internally), we send it over the wire.
	cs.sendVote(proposal, pc)
	return
}

// Propose creates a new outgoing proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) {
	cs.logger.Debugf("Propose")
	ctx, cancel := timeout.Context(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()
	// find a value to propose.
	// NOTE: this is blocking until a batch is present in the cache.
	cmd, ok := cs.commandCache.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[view=%d]: No command", view)
		return
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	proposal, ok := cs.ruler.ProposeRule(view, highQC, syncInfo, cmd)
	if !ok {
		cs.logger.Debug("Propose: No block")
		return
	}
	// TODO(AlanRostem): Why is here? Commented it out since it essentially stores the same block twice.
	// I added a panic in blockchain, as well. Seems to be the correct way.
	// cs.blockChain.Store(proposal.Block)
	// kauri sends the proposal to only the children
	if cs.kauri == nil {
		cs.sender.Propose(proposal)
	}
	// as proposer, I can vote for my own proposal
	cs.voteSelf(proposal)
}

// ProposeRule implements the default propose ruler.
func (cs *Consensus) ProposeRule(view hotstuff.View, _ hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	proposal = hotstuff.ProposeMsg{
		ID: cs.config.ID(),
		Block: hotstuff.NewBlock(
			qc.BlockHash(),
			qc,
			cmd,
			view,
			cs.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); ok && cs.config.HasAggregateQC() {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

func (cs *Consensus) voteSelf(proposal hotstuff.ProposeMsg) {
	if !cs.committer.TryCommit(proposal.Block) {
		return
	}
	block := proposal.Block
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
	err := cs.sender.SendVote(leaderID, pc)
	if err != nil {
		cs.logger.Warnf("%v", err)
		return
	}
	cs.logger.Debugf("TryVote[view=%d]: voting for %v", view, block)
}

var _ modules.ProposeRuler = (*Consensus)(nil)
