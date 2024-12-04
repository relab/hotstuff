package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/debug"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// Rules is the minimum interface that a consensus implementations must implement.
// Implementations of this interface can be wrapped in the ConsensusBase struct.
// Together, these provide an implementation of the main Consensus interface.
// Implementors do not need to verify certificates or interact with other modules,
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

// consensusBase provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type consensusBase struct {
	impl Rules

	acceptor       modules.Acceptor
	blockChain     modules.BlockChain
	committer      modules.Committer
	commandQueue   modules.CommandQueue
	configuration  modules.Configuration
	crypto         modules.Crypto
	eventLoop      *eventloop.ScopedEventLoop
	forkHandler    modules.ForkHandlerExt
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *modules.Options
	synchronizer   modules.Synchronizer

	handel modules.Handel

	lastVote hotstuff.View
	pipe     hotstuff.Pipe

	mut sync.Mutex
}

// New returns a new Consensus instance based on the given Rules implementation.
func New() modules.Consensus {
	return &consensusBase{
		lastVote: 0,
	}
}

// InitModule initializes the module.
func (cs *consensusBase) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	cs.pipe = info.ModuleScope

	mods.GetScoped(cs,
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
		&cs.synchronizer,
	)

	mods.TryGet(&cs.handel)

	// if mod, ok := cs.impl.(modules.Module); ok {
	// 	mod.InitModule(mods, initOpt)
	// }

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	}, eventloop.RespondToScope(info.ModuleScope))
}

func (cs *consensusBase) CommittedBlock() *hotstuff.Block {
	return cs.committer.CommittedBlock(cs.pipe)
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (cs *consensusBase) StopVoting(view hotstuff.View) {
	if cs.lastVote < view {
		cs.logger.Debugf("stopped voting on view %d and changed view to %d", cs.lastVote, view)
		cs.lastVote = view
	}
}

// Propose creates a new proposal.
func (cs *consensusBase) Propose(cert hotstuff.SyncInfo) {
	cs.logger.Debugf("Propose[p=%d]", cs.pipe)

	if cs.pipe != cert.Pipe() {
		panic("incorrect pipe")
	}

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		if qcBlock, ok := cs.blockChain.Get(qc.BlockHash(), cs.pipe); ok {
			cs.acceptor.Proposed(qcBlock.Command())
		} else {
			cs.logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := synchronizer.ScopedTimeoutContext(cs.eventLoop.Context(), cs.eventLoop, cs.pipe)
	defer cancel()

	cmd, ok := cs.commandQueue.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[p=%d, view=%d]: No command", cs.pipe, cs.synchronizer.View())
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.impl.(ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.logger.Debug("Propose[p=%d]: No block", cs.pipe)
			return
		}
	} else {
		proposal = hotstuff.ProposeMsg{
			ID: cs.opts.ID(),
			Block: hotstuff.NewBlock(
				qc.BlockHash(),
				qc,
				cmd,
				cs.synchronizer.View(),
				cs.opts.ID(),
				cs.pipe,
			),
			Pipe: cs.pipe,
		}

		if aggQC, ok := cert.AggQC(); ok && cs.opts.ShouldUseAggQC() {
			proposal.AggregateQC = &aggQC
		}
	}

	cs.blockChain.Store(proposal.Block)

	cs.configuration.Propose(proposal)
	// self vote
	cs.OnPropose(proposal)
}

func (cs *consensusBase) OnPropose(proposal hotstuff.ProposeMsg) { //nolint:gocyclo
	// TODO: extract parts of this method into helper functions maybe?
	cs.logger.Debugf("OnPropose[p=%d, view=%d]: %.8s -> %.8x", cs.pipe, cs.synchronizer.View(), proposal.Block.Hash(), proposal.Block.Command())
	if cs.pipe != proposal.Pipe {
		panic("OnPropose: incorrect pipe")
	}

	block := proposal.Block

	if cs.opts.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.crypto.VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.logger.Warnf("OnPropose[p=%d, view=%d]: failed to verify aggregate QC", cs.pipe, cs.synchronizer.View())
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.logger.Warnf("OnPropose[p=%d, view=%d]: block QC does not equal highQC", cs.pipe, cs.synchronizer.View())
			return
		}
	}

	if !cs.crypto.VerifyQuorumCert(block.QuorumCert()) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: invalid QC", cs.pipe, cs.synchronizer.View())
		return
	}

	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: block was not proposed by the expected leader", cs.pipe, cs.synchronizer.View())
		return
	}

	if !cs.impl.VoteRule(proposal) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: Block not voted for", cs.pipe, cs.synchronizer.View())
		return
	}

	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash(), cs.pipe); ok {
		cs.acceptor.Proposed(qcBlock.Command())
	} else {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: Failed to fetch qcBlock", cs.pipe, cs.synchronizer.View())
	}

	cmd := block.Command()
	if !cs.acceptor.Accept(cmd) {
		cs.logger.Infof("OnPropose[p=%d, view=%d]: block rejected: %.8s -> %.8x", cs.pipe, cs.synchronizer.View(), block.Hash(), block.Command())
		cs.eventLoop.DebugEvent(debug.CommandRejectedEvent{Pipe: cs.pipe, View: cs.synchronizer.View()})
		return
	}

	cs.logger.Debugf("OnPropose[p=%d, view=%d]: block accepted: %.8s -> %.8x", cs.pipe, cs.synchronizer.View(), block.Hash(), block.Command())

	// block is safe and was accepted
	cs.blockChain.Store(block)

	if b := cs.impl.CommitRule(block); b != nil {
		cs.committer.Commit(block)
	}
	cs.synchronizer.AdvanceView(hotstuff.NewSyncInfo(cs.pipe).WithQC(block.QuorumCert()))

	if block.View() <= cs.lastVote {
		cs.logger.Info(fmt.Sprintf("OnPropose[p=%d, view=%d]: block view too old for %.8s -> %.8x (diff=%d)", cs.pipe, cs.synchronizer.View(), block.Hash(), block.Command(), cs.lastVote-block.View()))
		return
	}

	pc, err := cs.crypto.CreatePartialCert(block)
	if err != nil {
		cs.logger.Errorf("OnPropose[p=%d, view=%d]: failed to sign block: ", cs.pipe, cs.synchronizer.View(), err)
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
		cs.eventLoop.AddScopedEvent(cs.pipe, hotstuff.VoteMsg{ID: cs.opts.ID(), PartialCert: pc})
		return
	}

	leader, ok := cs.configuration.Replica(leaderID)
	if !ok {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	cs.logger.Debugf("OnPropose[p=%d, view=%d]: voting for %.8s -> %.8x", cs.pipe, cs.synchronizer.View(), block.Hash(), block.Command())
	leader.Vote(pc)
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *consensusBase) ChainLength() int {
	return cs.impl.ChainLength()
}
