package consensus

import (
	"fmt"

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
	kauri          modules.Kauri
	leaderRotation modules.LeaderRotation

	blockChain   *blockchain.BlockChain
	committer    *committer.Committer
	commandCache *cmdcache.Cache
	sender       *network.Sender
	auth         *certauth.CertAuthority
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	config       *core.RuntimeConfig

	lastVote hotstuff.View
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

		lastVote: 0,
	}
	cs.ruler = cs
	for _, opt := range opts {
		opt(cs)
	}
	if config.KauriEnabled() {
		cs.kauri = kauri.New(
			auth,
			leaderRotation,
			blockChain,
			config,
			eventLoop,
			sender,
			logger,
			config.Tree(),
		)
	}
	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.logger.Debugf("On event (hotstuff.ProposeMsg) call OnPropose.")
		sInfo, advance := cs.OnPropose(event.(hotstuff.ProposeMsg))
		if advance {
			cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
				ID:       cs.config.ID(),
				SyncInfo: sInfo,
			})
		}
	})
	return cs
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (cs *Consensus) StopVoting(view hotstuff.View) {
	if cs.lastVote < view {
		cs.logger.Debugf("stopped voting on view %d and changed view to %d", cs.lastVote, view)
		cs.lastVote = view
	}
}

func (cs *Consensus) OnPropose(proposal hotstuff.ProposeMsg) (syncInfo hotstuff.SyncInfo, advance bool) {
	// TODO: extract parts of this method into helper functions maybe?
	block := proposal.Block
	if !cs.checkQC(&proposal) {
		return
	}
	if !cs.acceptProposal(&proposal) {
		return
	}
	view := block.View()
	cs.logger.Debugf("OnPropose[view=%d]: block accepted.", view)
	cs.blockChain.Store(block)
	if b := cs.impl.CommitRule(block); b != nil {
		cs.committer.Commit(b)
	}
	// Tells the synchronizer to advance the view
	advance = true
	syncInfo = hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	if block.View() <= cs.lastVote {
		cs.logger.Info(fmt.Sprintf("OnPropose[view=%d]: block view too old for.", view))
		return
	}
	cs.voteFor(&proposal)
	return
}

// Propose creates a new proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo) {
	cs.logger.Debugf("Propose")

	// TODO(AlanRostem): why the previous proposal need to be marked as accepted? Shouldn't this happen on execution?
	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		if qcBlock, ok := cs.blockChain.Get(qc.BlockHash()); ok {
			cs.commandCache.Proposed(qcBlock.Command())
		} else {
			cs.logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := timeout.Context(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()

	cmd, ok := cs.commandCache.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[view=%d]: No command", view)
		return
	}

	var proposal hotstuff.ProposeMsg
	proposal, ok = cs.ruler.ProposeRule(view, highQC, cert, cmd)
	if !ok {
		cs.logger.Debug("Propose: No block")
		return
	}

	// TODO(AlanRostem): discuss if this removal is valid. Adding a panic in blockchain seems to be the correct way.
	// cs.blockChain.Store(proposal.Block)
	// kauri sends the proposal to only the children
	if cs.kauri == nil {
		cs.sender.Propose(proposal)
	}
	// self vote - can discard return value since the syncinfo is already the recent one
	cs.OnPropose(proposal)
}

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

func (cs *Consensus) checkQC(proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	view := block.View()
	if cs.config.HasAggregateQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.auth.VerifyAggregateQC(cs.config.QuorumSize(), *proposal.AggregateQC)
		if !ok {
			cs.logger.Warnf("checkQC[view=%d]: failed to verify aggregate QC", view)
			return false
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.logger.Warnf("checkQC[view=%d]: block QC does not equal highQC", view)
			return false
		}
	}
	if !cs.auth.VerifyQuorumCert(cs.config.QuorumSize(), block.QuorumCert()) {
		cs.logger.Infof("checkQC[view=%d]: invalid QC", view)
		return false
	}
	return true
}

func (cs *Consensus) acceptProposal(proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	view := block.View()
	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: block was not proposed by the expected leader", view)
		return false
	}
	if !cs.impl.VoteRule(view, *proposal) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: Block not voted for", view)
		return false
	}
	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.commandCache.Proposed(qcBlock.Command())
	} else {
		cs.logger.Infof("OnPropose[view=%d]: Failed to fetch qcBlock", view)
	}
	cmd := block.Command()
	if !cs.commandCache.Accept(cmd) {
		cs.logger.Infof("OnPropose[view=%d]: block rejected: %.8s -> %.8x", view, block.Hash(), block.Command())
		return false
	}
	return true
}

func (cs *Consensus) voteFor(proposal *hotstuff.ProposeMsg) {
	block := proposal.Block
	view := block.View()
	pc, err := cs.auth.CreatePartialCert(block)
	if err != nil {
		cs.logger.Errorf("OnPropose[view=%d]: failed to sign block: ", view, err)
		return
	}
	cs.lastVote = block.View()
	if cs.kauri != nil {
		cs.kauri.Begin(pc, *proposal)
		return
	}
	leaderID := cs.leaderRotation.GetLeader(cs.lastVote + 1)
	if leaderID == cs.config.ID() {
		cs.eventLoop.AddEvent(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
		return
	}
	if !cs.sender.ReplicaExists(leaderID) {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}
	cs.logger.Debugf("OnPropose[view=%d]: voting for %.8s -> %.8x", view, block.Hash(), block.Command())
	cs.sender.SendVote(leaderID, pc)
}

var _ modules.ProposeRuler = (*Consensus)(nil)
