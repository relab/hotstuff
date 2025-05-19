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
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/committer"
)

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	impl           modules.ConsensusRules
	ruler          modules.ProposeRuler
	kauri          *kauri.Kauri
	leaderRotation modules.LeaderRotation

	blockChain   *blockchain.BlockChain
	committer    *committer.Committer
	commandCache *cmdcache.Cache
	sender       *network.Sender
	auth         *certauth.CertAuthority
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	config       *core.RuntimeConfig

	voter  *Voter
	leader *Leader
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
	voter *Voter,
	leader *Leader,

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

		blockChain:   blockChain,
		committer:    committer,
		commandCache: commandCache,
		sender:       sender,
		auth:         auth,
		eventLoop:    eventLoop,
		logger:       logger,
		config:       config,

		voter:  voter,
		leader: leader,
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
		if !cs.ProcessProposal(proposal) {
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

func (cs *Consensus) ProcessProposal(proposal hotstuff.ProposeMsg) (advance bool) {
	block := proposal.Block
	view := block.View()
	advance = false // won't increment the view when the below if-statements return
	// ensure that I can vote in this view based on the protocol's rule.
	if !cs.impl.VoteRule(view, proposal) {
		cs.logger.Info("OnPropose: Block not voted for")
		return
	}
	if !cs.voter.TryAccept(&proposal) {
		return
	}
	// now the proposal is valid, so we can increment the view, but the command may
	// still be too old to execute
	if !cs.tryCache(&proposal) {
		return
	}

	cs.tryCommit(&proposal)
	advance = true // Tells the synchronizer to advance the view, even if vote creation failed.
	// try to vote for the block and retrieve its partial certificate.
	pc, ok := cs.voter.CreateVote(block)
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

func (cs *Consensus) tryCommit(proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	view := block.View()
	cs.logger.Debugf("tryCommit[view=%d]: block accepted.", view)
	cs.blockChain.Store(block)
	// TODO(AlanRostem): verify if this is the line below is a solution to this comment: https://github.com/relab/hotstuff/pull/182/files#r1932600780
	cs.commandCache.Update(block.Command()) // update the cache before committing.
	// NOTE: this overwrites the block variable. If it was nil, dont't commit.
	if block = cs.impl.CommitRule(block); block == nil {
		return false
	}
	cs.committer.Commit(block) // committer will eventually execute the command.
	return true
}

// TODO(AlanRostem): find a better name for this method.
func (cs *Consensus) tryCache(proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	view := block.View()
	cmd := block.Command()
	// verify the command's "age"
	if !cs.commandCache.Accept(cmd) {
		cs.logger.Infof("TryAccept[view=%d]: block rejected: %s", view, block)
		return false
	}
	// ensure that the block's QC is present on the chain.
	// if it is, then we tell the cmdcache to update its timeline.
	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.commandCache.Update(qcBlock.Command())
	} else {
		cs.logger.Infof("TryAccept[view=%d]: Failed to fetch qcBlock", view)
	}
	return true
}

// Propose creates a new proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) {
	cs.logger.Debugf("Propose")

	// TODO(AlanRostem): related to this comment: https://github.com/relab/hotstuff/pull/182/files#r1932600780
	// I removed this and moved the update directory to tryCommit. Seems to work smoothly.
	// qc, ok := syncInfo.QC()
	// if ok {
	// 	// tell the acceptor that the previous proposal succeeded.
	// 	if qcBlock, ok := cs.blockChain.Get(qc.BlockHash()); ok {
	// 		cs.commandCache.Update(qcBlock.Command())
	// 	} else {
	// 		cs.logger.Errorf("Could not find block for QC: %s", qc)
	// 	}
	// }

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
	var proposal hotstuff.ProposeMsg
	proposal, ok = cs.ruler.ProposeRule(view, highQC, syncInfo, cmd)
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
	// as leader, I can commit and vote for my own proposal
	// TODO(AlanRostem): can this be avoided and just vote + commit directly as leader?
	cs.ProcessProposal(proposal)
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

func (cs *Consensus) sendVote(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	block := proposal.Block
	view := block.View()
	// if I am the leader in the next view, collect the vote for myself beforehand.
	leaderID := cs.leaderRotation.GetLeader(cs.voter.LastVote() + 1)
	if leaderID == cs.config.ID() {
		cs.leader.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
		return
	}
	// if I am the one voting, sent the vote to next leader over the wire.
	if !cs.sender.ReplicaExists(leaderID) {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}
	cs.logger.Debugf("TryVote[view=%d]: voting for %v", view, block)
	cs.sender.SendVote(leaderID, pc)
}

var _ modules.ProposeRuler = (*Consensus)(nil)
