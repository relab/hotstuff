package consensus

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipelining"
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
	blockComp      modules.BlockCommitter
	commandQueue   modules.CommandQueue
	configuration  modules.Configuration
	crypto         modules.Crypto
	eventLoop      *eventloop.EventLoop
	forkHandler    modules.ForkHandlerExt
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *modules.Options
	synchronizer   modules.Synchronizer

	handel modules.Handel

	lastVote hotstuff.View
	pipe     pipelining.PipeId

	mut sync.Mutex
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(impl Rules) modules.Consensus {
	return &consensusBase{
		impl:     impl,
		lastVote: 0,
	}
}

// InitModule initializes the module.
func (cs *consensusBase) InitModule(mods *modules.Core, initOpt modules.InitOptions) {
	cs.pipe = initOpt.ModulePipeId

	mods.Get(
		&cs.acceptor,
		&cs.blockChain,
		&cs.blockComp,
		&cs.commandQueue,
		&cs.configuration,
		&cs.crypto,
		&cs.eventLoop,
		&cs.forkHandler,
		&cs.logger,
		&cs.opts,
	)

	mods.GetFromPipe(cs,
		&cs.synchronizer,
		&cs.leaderRotation,
	)

	mods.TryGet(&cs.handel)

	if mod, ok := cs.impl.(modules.Module); ok {
		mod.InitModule(mods, initOpt)
	}

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	}, eventloop.RespondToPipe(initOpt.ModulePipeId))
}

func (cs *consensusBase) CommittedBlock() *hotstuff.Block {
	return cs.blockComp.CommittedBlock(cs.pipe)
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (cs *consensusBase) StopVoting(view hotstuff.View) {
	if cs.lastVote < view {
		cs.lastVote = view
	}
}

// Propose creates a new proposal.
func (cs *consensusBase) Propose(cert hotstuff.SyncInfo) {
	cs.logger.Debug("Propose")

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
		cs.logger.Debug("Propose: No command")
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
				cs.synchronizer.View(),
				cs.opts.ID(),
				0,
			),
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
	cs.logger.Debugf("OnPropose: %v", proposal.Block)

	block := proposal.Block

	if cs.opts.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.crypto.VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.logger.Warn("OnPropose: failed to verify aggregate QC")
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.logger.Warn("OnPropose: block QC does not equal highQC")
			return
		}
	}

	if !cs.crypto.VerifyQuorumCert(block.QuorumCert()) {
		cs.logger.Info("OnPropose: invalid QC")
		return
	}

	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if !cs.impl.VoteRule(proposal) {
		cs.logger.Info("OnPropose: Block not voted for")
		return
	}

	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.acceptor.Proposed(qcBlock.Command())
	} else {
		cs.logger.Info("OnPropose: Failed to fetch qcBlock")
	}

	if !cs.acceptor.Accept(block.Command()) {
		cs.logger.Info("OnPropose: command not accepted")
		return
	}

	// block is safe and was accepted
	cs.blockChain.Store(block)

	if b := cs.impl.CommitRule(block); b != nil {
		cs.blockComp.Store(block)
	}
	cs.synchronizer.AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))

	if block.View() <= cs.lastVote {
		cs.logger.Info("OnPropose: block view too old")
		return
	}

	pc, err := cs.crypto.CreatePartialCert(block)
	if err != nil {
		cs.logger.Error("OnPropose: failed to sign block: ", err)
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

	leader.Vote(pc)
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *consensusBase) ChainLength() int {
	return cs.impl.ChainLength()
}
