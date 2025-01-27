package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/certauth"
	"github.com/relab/hotstuff/clientsrv"
	"github.com/relab/hotstuff/committer"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/kauri"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/sender"
	"github.com/relab/hotstuff/synctools"
)

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	impl           modules.Rules
	leaderRotation modules.LeaderRotation

	blockChain   *blockchain.BlockChain
	committer    *committer.Committer
	commandCache *clientsrv.CmdCache
	sender       *sender.Sender
	auth         *certauth.CertAuthority
	eventLoop    *core.EventLoop
	logger       logging.Logger
	opts         *core.Options

	kauri modules.Kauri

	lastVote hotstuff.View
	view     hotstuff.View

	mut sync.Mutex
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(
	impl modules.Rules,
	leaderRotation modules.LeaderRotation,

	blockChain *blockchain.BlockChain,
	committer *committer.Committer,
	commandCache *clientsrv.CmdCache,
	sender *sender.Sender,
	kauri *kauri.Kauri,
	auth *certauth.CertAuthority,
	eventLoop *core.EventLoop,
	logger logging.Logger,
	opts *core.Options,
) *Consensus {
	cs := &Consensus{
		impl:           impl,
		leaderRotation: leaderRotation,

		blockChain:   blockChain,
		committer:    committer,
		commandCache: commandCache,
		sender:       sender,
		auth:         auth,
		eventLoop:    eventLoop,
		logger:       logger,
		opts:         opts,

		lastVote: 0,
	}

	if kauri != nil {
		cs.kauri = kauri
	}

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(cs.view, event.(hotstuff.ProposeMsg))
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

// Propose creates a new proposal.
func (cs *Consensus) Propose(view hotstuff.View, cert hotstuff.SyncInfo) (syncInfo hotstuff.SyncInfo, advance bool) {
	cs.logger.Debugf("Propose")
	cs.view = view

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		if qcBlock, ok := cs.blockChain.Get(qc.BlockHash()); ok {
			cs.commandCache.Proposed(qcBlock.Command())
		} else {
			cs.logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := synctools.TimeoutContext(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()

	cmd, ok := cs.commandCache.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[view=%d]: No command", view)
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.impl.(modules.ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.logger.Debug("Propose: No block")
			return
		}
	} else {
		proposal = hotstuff.ProposeMsg{
			ID: cs.opts.ID(),
			Block: hotstuff.NewBlock(
				qc.BlockHash(),
				qc,
				cmd,
				view,
				cs.opts.ID(),
			),
		}

		if aggQC, ok := cert.AggQC(); ok && cs.opts.ShouldUseAggQC() {
			proposal.AggregateQC = &aggQC
		}
	}

	cs.blockChain.Store(proposal.Block)
	// kauri sends the proposal to only the children
	if cs.kauri == nil {
		cs.sender.Propose(proposal)
	}
	// self vote
	return cs.OnPropose(view, proposal)
}

func (cs *Consensus) checkQC(view hotstuff.View, proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	if cs.opts.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.auth.VerifyAggregateQC(*proposal.AggregateQC)
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
	if !cs.auth.VerifyQuorumCert(block.QuorumCert()) {
		cs.logger.Infof("checkQC[view=%d]: invalid QC", view)
		return false
	}
	return true
}

func (cs *Consensus) acceptProposal(view hotstuff.View, proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
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

func (cs *Consensus) voteFor(view hotstuff.View, proposal *hotstuff.ProposeMsg) {
	block := proposal.Block
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
	if leaderID == cs.opts.ID() {
		cs.eventLoop.AddEvent(hotstuff.VoteMsg{ID: cs.opts.ID(), PartialCert: pc})
		return
	}
	leader, ok := cs.sender.ReplicaNode(leaderID)
	if !ok {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}
	cs.logger.Debugf("OnPropose[view=%d]: voting for %.8s -> %.8x", view, block.Hash(), block.Command())
	leader.Vote(pc)
}

func (cs *Consensus) OnPropose(view hotstuff.View, proposal hotstuff.ProposeMsg) (syncInfo hotstuff.SyncInfo, advance bool) {
	// TODO: extract parts of this method into helper functions maybe?
	cs.logger.Debugf("OnPropose[view=%d]: %.8s -> %.8x", view, proposal.Block.Hash(), proposal.Block.Command())
	block := proposal.Block
	if !cs.checkQC(view, &proposal) {
		return
	}
	if !cs.acceptProposal(view, &proposal) {
		return
	}
	cs.logger.Debugf("OnPropose[view=%d]: block accepted: %.8s -> %.8x", view, block.Hash(), block.Command())
	// block is safe and was accepted
	cs.blockChain.Store(block)
	if b := cs.impl.CommitRule(block); b != nil {
		cs.committer.Commit(b)
	}
	// Tells the synchronizer to advance the view
	advance = true
	syncInfo = hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	if block.View() <= cs.lastVote {
		cs.logger.Info(fmt.Sprintf("OnPropose[view=%d]: block view too old for %.8s -> %.8x (diff=%d)", view, block.Hash(), block.Command(), cs.lastVote-block.View()))
		return
	}
	cs.voteFor(view, &proposal)
	return
}

// SetByzantine wraps the existing rule implementation to a byzantine ruleset.
func (cs *Consensus) SetByzantine(byzRules modules.Rules) {
	cs.impl = byzRules
}

var _ core.Consensus = (*Consensus)(nil)
