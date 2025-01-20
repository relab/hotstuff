package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/committer"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// Rules is the minimum interface that a consensus implementations must implement.
// Implementations of this interface can be wrapped in the ConsensusBase struct.
// Together, these provide an implementation of the main Consensus interface.
// Implementors do not need to verify certificates or interact with other core,
// as this is handled by the ConsensusBase struct.
type Rules interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(proposal hotstuff.ProposeMsg) bool
	// CommitRule decides whether any ancestor of the block can be committed.
	// Returns the youngest ancestor of the block that can be committed.
	CommitRule(*hotstuff.Block) *hotstuff.Block
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

// ProposeRuler is an optional interface that adds a ProposeRule method.
// This allows implementors to specify how new blocks are created.
type ProposeRuler interface {
	// ProposeRule creates a new proposal.
	ProposeRule(cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool)
}

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	impl Rules

	acceptor       core.Acceptor
	blockChain     core.BlockChain
	committer      *committer.Committer
	commandQueue   core.CommandQueue
	configuration  core.Configuration
	crypto         core.Crypto
	eventLoop      *core.EventLoop
	forkHandler    core.ForkHandlerExt
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *core.Options

	handel core.Handel

	lastVote hotstuff.View
	view     hotstuff.View

	mut sync.Mutex
}

// New returns a new Consensus instance based on the given Rules implementation.
func New() *Consensus {
	return &Consensus{
		lastVote: 0,
	}
}

// InitModule initializes the module.
func (cs *Consensus) InitModule(mods *core.Core) {
	mods.Get(
		&cs.acceptor,
		&cs.blockChain,
		&cs.commandQueue,
		&cs.committer,
		&cs.configuration,
		&cs.crypto,
		&cs.eventLoop,
		&cs.forkHandler,
		&cs.leaderRotation,
		&cs.logger,
		&cs.opts,
		&cs.impl,
	)

	mods.TryGet(&cs.handel)

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		// TODO: determine potential bugs
		cs.OnPropose(cs.view, event.(hotstuff.ProposeMsg))
	})
}

func (cs *Consensus) CommittedBlock() *hotstuff.Block {
	return cs.committer.CommittedBlock()
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
			cs.acceptor.Proposed(qcBlock.Command())
		} else {
			cs.logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := synchronizer.TimeoutContext(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()

	cmd, ok := cs.commandQueue.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[view=%d]: No command", view)
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.impl.(ProposeRuler); ok {
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

	cs.configuration.Propose(proposal)
	// self vote
	return cs.OnPropose(view, proposal)
}

func (cs *Consensus) OnPropose(view hotstuff.View, proposal hotstuff.ProposeMsg) (syncInfo hotstuff.SyncInfo, advance bool) { //nolint:gocyclo
	// TODO: extract parts of this method into helper functions maybe?
	cs.logger.Debugf("OnPropose[view=%d]: %.8s -> %.8x", view, proposal.Block.Hash(), proposal.Block.Command())

	block := proposal.Block

	if cs.opts.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.crypto.VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.logger.Warnf("OnPropose[view=%d]: failed to verify aggregate QC", view)
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.logger.Warnf("OnPropose[view=%d]: block QC does not equal highQC", view)
			return
		}
	}

	if !cs.crypto.VerifyQuorumCert(block.QuorumCert()) {
		cs.logger.Infof("OnPropose[view=%d]: invalid QC", view)
		return
	}

	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: block was not proposed by the expected leader", view)
		return
	}

	if !cs.impl.VoteRule(proposal) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: Block not voted for", view)
		return
	}

	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.acceptor.Proposed(qcBlock.Command())
	} else {
		cs.logger.Infof("OnPropose[view=%d]: Failed to fetch qcBlock", view)
	}

	cmd := block.Command()
	if !cs.acceptor.Accept(cmd) {
		cs.logger.Infof("OnPropose[view=%d]: block rejected: %.8s -> %.8x", view, block.Hash(), block.Command())
		return
	}

	cs.logger.Debugf("OnPropose[view=%d]: block accepted: %.8s -> %.8x", view, block.Hash(), block.Command())

	// block is safe and was accepted
	cs.blockChain.Store(block)

	if b := cs.impl.CommitRule(block); b != nil {
		cs.committer.Commit(view, block)
	}

	// Tells the synchronizer to advance the view
	advance = true
	syncInfo = hotstuff.NewSyncInfo().WithQC(block.QuorumCert())

	if block.View() <= cs.lastVote {
		cs.logger.Info(fmt.Sprintf("OnPropose[view=%d]: block view too old for %.8s -> %.8x (diff=%d)", view, block.Hash(), block.Command(), cs.lastVote-block.View()))
		return
	}

	pc, err := cs.crypto.CreatePartialCert(block)
	if err != nil {
		cs.logger.Errorf("OnPropose[view=%d]: failed to sign block: ", view, err)
		return
	}

	cs.lastVote = block.View()

	if cs.handel != nil {
		// let Handel handle the voting
		cs.handel.Begin(pc)
		return
	}

	leaderID := cs.leaderRotation.GetLeader(cs.lastVote + 1)
	if leaderID == cs.opts.ID() {
		cs.eventLoop.AddEvent(hotstuff.VoteMsg{ID: cs.opts.ID(), PartialCert: pc})
		return
	}

	leader, ok := cs.configuration.Replica(leaderID)
	if !ok {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	cs.logger.Debugf("OnPropose[view=%d]: voting for %.8s -> %.8x", view, block.Hash(), block.Command())
	leader.Vote(pc)
	return
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *Consensus) ChainLength() int {
	return cs.impl.ChainLength()
}

var _ core.Consensus = (core.Consensus)(nil)
