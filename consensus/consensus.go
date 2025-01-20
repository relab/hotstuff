package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// consensusBase provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type consensusBase struct {
	rules          modules.Rules
	leaderRotation modules.LeaderRotation

	comps core.ComponentList

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
	cs.comps = mods.Components()

	mods.Get(
		&cs.leaderRotation,
		&cs.rules,
	)

	if mod, ok := cs.rules.(core.Component); ok {
		mod.InitComponent(mods)
	}

	cs.comps.EventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
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
	cs.comps.Logger.Debug("Propose")

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		if qcBlock, ok := cs.comps.BlockChain.Get(qc.BlockHash()); ok {
			cs.comps.CommandCache.Proposed(qcBlock.Command())
		} else {
			cs.comps.Logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := synchronizer.TimeoutContext(cs.comps.EventLoop.Context(), cs.comps.EventLoop)
	defer cancel()

	cmd, ok := cs.comps.CommandCache.Get(ctx)
	if !ok {
		cs.comps.Logger.Debug("Propose: No command")
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.rules.(modules.ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.comps.Logger.Debug("Propose: No block")
			return
		}
	} else {
		proposal = hotstuff.ProposeMsg{
			ID: cs.comps.Options.ID(),
			Block: hotstuff.NewBlock(
				qc.BlockHash(),
				qc,
				cmd,
				cs.comps.Synchronizer.View(),
				cs.comps.Options.ID(),
			),
		}

		if aggQC, ok := cert.AggQC(); ok && cs.comps.Options.ShouldUseAggQC() {
			proposal.AggregateQC = &aggQC
		}
	}

	cs.comps.BlockChain.Store(proposal.Block)

	cs.comps.Configuration.Propose(proposal)
	// self vote
	cs.OnPropose(proposal)
}

func (cs *consensusBase) OnPropose(proposal hotstuff.ProposeMsg) { //nolint:gocyclo
	// TODO: extract parts of this method into helper functions maybe?
	cs.comps.Logger.Debugf("OnPropose: %v", proposal.Block)

	block := proposal.Block

	if cs.comps.Options.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.comps.Crypto.VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.comps.Logger.Warn("OnPropose: failed to verify aggregate QC")
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.comps.Logger.Warn("OnPropose: block QC does not equal highQC")
			return
		}
	}

	if !cs.comps.Crypto.VerifyQuorumCert(block.QuorumCert()) {
		cs.comps.Logger.Info("OnPropose: invalid QC")
		return
	}

	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.comps.Logger.Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if !cs.rules.VoteRule(proposal) {
		cs.comps.Logger.Info("OnPropose: Block not voted for")
		return
	}

	if qcBlock, ok := cs.comps.BlockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.comps.CommandCache.Proposed(qcBlock.Command())
	} else {
		cs.comps.Logger.Info("OnPropose: Failed to fetch qcBlock")
	}

	if !cs.comps.CommandCache.Accept(block.Command()) {
		cs.comps.Logger.Info("OnPropose: command not accepted")
		return
	}

	// block is safe and was accepted
	cs.comps.BlockChain.Store(block)

	if b := cs.rules.CommitRule(block); b != nil {
		cs.commit(b)
	}
	cs.comps.Synchronizer.AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))

	if block.View() <= cs.lastVote {
		cs.comps.Logger.Info("OnPropose: block view too old")
		return
	}

	pc, err := cs.comps.Crypto.CreatePartialCert(block)
	if err != nil {
		cs.comps.Logger.Error("OnPropose: failed to sign block: ", err)
		return
	}

	cs.lastVote = block.View()

	leaderID := cs.leaderRotation.GetLeader(cs.lastVote + 1)
	if leaderID == cs.comps.Options.ID() {
		cs.comps.EventLoop.AddEvent(hotstuff.VoteMsg{ID: cs.comps.Options.ID(), PartialCert: pc})
		return
	}

	leader, ok := cs.comps.Configuration.Replica(leaderID)
	if !ok {
		cs.comps.Logger.Warnf("Replica with ID %d was not found!", leaderID)
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
		cs.comps.Logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	forkedBlocks := cs.comps.BlockChain.PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		cs.comps.ForkHandler.Fork(block)
	}
}

// recursive helper for commit
func (cs *consensusBase) commitInner(block *hotstuff.Block) error {
	if cs.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := cs.comps.BlockChain.Get(block.Parent()); ok {
		err := cs.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cs.comps.Logger.Debug("EXEC: ", block)
	cs.comps.Executor.Exec(block)
	cs.bExec = block
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *consensusBase) ChainLength() int {
	return cs.rules.ChainLength()
}
