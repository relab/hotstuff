package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
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
	mods *modules.ConsensusCore

	lastVote hotstuff.View

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(impl Rules) modules.Consensus {
	return &consensusBase{
		impl:     impl,
		lastVote: 0,
		bExec:    hotstuff.GetGenesis(),
	}
}

func (cs *consensusBase) CommittedBlock() *hotstuff.Block {
	cs.mut.Lock()
	defer cs.mut.Unlock()
	return cs.bExec
}

func (cs *consensusBase) InitModule(mods *modules.ConsensusCore, opts *modules.OptionsBuilder) {
	cs.mods = mods
	if mod, ok := cs.impl.(modules.ConsensusModule); ok {
		mod.InitModule(mods, opts)
	}
	cs.mods.EventLoop().RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (cs *consensusBase) StopVoting(view hotstuff.View) {
	if cs.lastVote < view {
		cs.lastVote = view
	}
}

// Propose creates a new proposal.
func (cs *consensusBase) Propose(cert hotstuff.SyncInfo) {
	cs.mods.Logger().Debug("Propose")

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		qcBlock, ok := cs.mods.BlockChain().Get(qc.BlockHash())
		if !ok {
			cs.mods.Logger().Errorf("Could not find block for QC: %s", qc)
			return
		}
		cs.mods.Acceptor().Proposed(qcBlock.Command())
	}

	cmd, ok := cs.mods.CommandQueue().Get(cs.mods.Synchronizer().ViewContext())
	if !ok {
		cs.mods.Logger().Debug("Propose: No command")
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.impl.(ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.mods.Logger().Debug("Propose: No block")
			return
		}
	} else {
		proposal = hotstuff.ProposeMsg{
			ID: cs.mods.ID(),
			Block: hotstuff.NewBlock(
				cs.mods.Synchronizer().LeafBlock().Hash(),
				qc,
				cmd,
				cs.mods.Synchronizer().View(),
				cs.mods.ID(),
			),
		}

		if aggQC, ok := cert.AggQC(); ok && cs.mods.Options().ShouldUseAggQC() {
			proposal.AggregateQC = &aggQC
		}
	}

	cs.mods.BlockChain().Store(proposal.Block)

	cs.mods.Configuration().Propose(proposal)
	// self vote
	cs.OnPropose(proposal)
}

func (cs *consensusBase) OnPropose(proposal hotstuff.ProposeMsg) { //nolint:gocyclo
	// TODO: extract parts of this method into helper functions maybe?
	cs.mods.Logger().Debugf("OnPropose: %v", proposal.Block)

	block := proposal.Block

	if cs.mods.Options().ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.mods.Crypto().VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.mods.Logger().Warn("OnPropose: failed to verify aggregate QC")
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.mods.Logger().Warn("OnPropose: block QC does not equal highQC")
			return
		}
	}

	if !cs.mods.Crypto().VerifyQuorumCert(block.QuorumCert()) {
		cs.mods.Logger().Info("OnPropose: invalid QC")
		return
	}

	cs.mods.Synchronizer().UpdateHighQC(block.QuorumCert())

	// ensure the block came from the leader.
	if proposal.ID != cs.mods.LeaderRotation().GetLeader(block.View()) {
		cs.mods.Logger().Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if !cs.impl.VoteRule(proposal) {
		cs.mods.Logger().Info("OnPropose: Block not voted for")
		return
	}

	if qcBlock, ok := cs.mods.BlockChain().Get(block.QuorumCert().BlockHash()); ok {
		cs.mods.Acceptor().Proposed(qcBlock.Command())
	} else {
		cs.mods.Logger().Info("OnPropose: Failed to fetch qcBlock")
	}

	if !cs.mods.Acceptor().Accept(block.Command()) {
		cs.mods.Logger().Info("OnPropose: command not accepted")
		return
	}

	// block is safe and was accepted
	cs.mods.BlockChain().Store(block)

	didAdvanceView := false
	// we defer the following in order to speed up voting
	defer func() {
		if b := cs.impl.CommitRule(block); b != nil {
			cs.commit(b)
		}
		if !didAdvanceView {
			cs.mods.Synchronizer().AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))
		}
	}()

	if block.View() <= cs.lastVote {
		cs.mods.Logger().Info("OnPropose: block view too old")
		return
	}

	pc, err := cs.mods.Crypto().CreatePartialCert(block)
	if err != nil {
		cs.mods.Logger().Error("OnPropose: failed to sign vote: ", err)
		return
	}

	cs.lastVote = block.View()

	if cs.mods.Options().ShouldUseHandel() {
		// Need to call advanceview such that the view context will be fresh.
		// TODO: we could instead
		cs.mods.Synchronizer().AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))
		didAdvanceView = true
		cs.mods.Handel().Begin(pc)
		return
	}

	leaderID := cs.mods.LeaderRotation().GetLeader(cs.lastVote + 1)
	if leaderID == cs.mods.ID() {
		cs.mods.EventLoop().AddEvent(hotstuff.VoteMsg{ID: cs.mods.ID(), PartialCert: pc})
		return
	}

	leader, ok := cs.mods.Configuration().Replica(leaderID)
	if !ok {
		cs.mods.Logger().Warnf("Replica with ID %d was not found!", leaderID)
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
		cs.mods.Logger().Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	forkedBlocks := cs.mods.BlockChain().PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		cs.mods.ForkHandler().Fork(block)
	}
}

// recursive helper for commit
func (cs *consensusBase) commitInner(block *hotstuff.Block) error {
	if cs.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := cs.mods.BlockChain().Get(block.Parent()); ok {
		err := cs.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cs.mods.Logger().Debug("EXEC: ", block)
	cs.mods.Executor().Exec(block)
	cs.bExec = block
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (cs *consensusBase) ChainLength() int {
	return cs.impl.ChainLength()
}
