package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// consensusBase provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type consensusBase struct {
	acceptor       core.Acceptor
	blockChain     core.BlockChain
	commandQueue   core.CommandQueue
	configuration  core.Configuration
	crypto         core.Crypto
	eventLoop      *eventloop.EventLoop
	executor       core.ExecutorExt
	forkHandler    core.ForkHandlerExt
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *core.Options
	rules          modules.Rules
	synchronizer   core.Synchronizer

	handel core.Handel

	lastVote hotstuff.View

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// New returns a new Consensus instance based on the given Rules implementation.
func New() core.Consensus {
	return &consensusBase{
		lastVote: 0,
		bExec:    hotstuff.GetGenesis(),
	}
}

// InitComponent initializes the module.
func (cs *consensusBase) InitComponent(mods *core.Core) {
	mods.Get(
		&cs.acceptor,
		&cs.blockChain,
		&cs.commandQueue,
		&cs.configuration,
		&cs.crypto,
		&cs.eventLoop,
		&cs.executor,
		&cs.forkHandler,
		&cs.rules,
		&cs.leaderRotation,
		&cs.logger,
		&cs.opts,
		&cs.synchronizer,
	)

	mods.TryGet(&cs.handel)

	if mod, ok := cs.rules.(core.Component); ok {
		mod.InitComponent(mods)
	}

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	})
}

func (cs *consensusBase) CommittedBlock() *hotstuff.Block {
	cs.mut.Lock()
	defer cs.mut.Unlock()
	return cs.bExec
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
	if proposer, ok := cs.rules.(modules.ProposeRuler); ok {
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

	if !cs.rules.VoteRule(proposal) {
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

	if b := cs.rules.CommitRule(block); b != nil {
		cs.commit(b)
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

func (cs *consensusBase) commit(block *hotstuff.Block) {
	cs.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := cs.commitInner(block)
	cs.mut.Unlock()

	if err != nil {
		cs.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	forkedBlocks := cs.blockChain.PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		cs.forkHandler.Fork(block)
	}
}

// recursive helper for commit
func (cs *consensusBase) commitInner(block *hotstuff.Block) error {
	if cs.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := cs.blockChain.Get(block.Parent()); ok {
		err := cs.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cs.logger.Debug("EXEC: ", block)
	cs.executor.Exec(block)
	cs.bExec = block
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *consensusBase) ChainLength() int {
	return cs.rules.ChainLength()
}
