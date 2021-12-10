package consensus

import (
	"fmt"
	"sync"
)

// Rules is the minimum interface that a consensus implementations must implement.
// Implementations of this interface can be wrapped in the ConsensusBase struct.
// Together, these provide an implementation of the main Consensus interface.
// Implementors do not need to verify certificates or interact with other modules,
// as this is handled by the ConsensusBase struct.
type Rules interface {
	// VoteRule decides whether to vote for the block.
	VoteRule(proposal ProposeMsg) bool
	// CommitRule decides whether any ancestor of the block can be committed.
	// Returns the youngest ancestor of the block that can be committed.
	CommitRule(*Block) *Block
}

// ProposeRuler is an optional interface that adds a ProposeRule method.
// This allows implementors to specify how new blocks are created.
type ProposeRuler interface {
	// ProposeRule creates a new proposal.
	ProposeRule(cert SyncInfo, cmd Command) (proposal ProposeMsg, ok bool)
}

// consensusBase provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type consensusBase struct {
	impl Rules
	mods *Modules

	lastVote View

	mut   sync.Mutex
	bExec *Block
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(impl Rules) Consensus {
	return &consensusBase{
		impl:     impl,
		lastVote: 0,
		bExec:    GetGenesis(),
	}
}

func (cs *consensusBase) CommittedBlock() *Block {
	cs.mut.Lock()
	defer cs.mut.Unlock()
	return cs.bExec
}

func (cs *consensusBase) InitConsensusModule(mods *Modules, opts *OptionsBuilder) {
	cs.mods = mods
	if mod, ok := cs.impl.(Module); ok {
		mod.InitConsensusModule(mods, opts)
	}
	cs.mods.EventLoop().RegisterHandler(ProposeMsg{}, func(event interface{}) {
		cs.OnPropose(event.(ProposeMsg))
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (cs *consensusBase) StopVoting(view View) {
	if cs.lastVote < view {
		cs.lastVote = view
	}
}

// Propose creates a new proposal.
func (cs *consensusBase) Propose(cert SyncInfo) {
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
	//fmt.Println("Command", cmd, "Bool", ok)
	if !ok {
		cs.mods.Logger().Debug("Propose: No command")
		return
	}

	var proposal ProposeMsg
	if proposer, ok := cs.impl.(ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.mods.Logger().Debug("Propose: No block")
			return
		}
	} else {
		proposal = ProposeMsg{
			ID: cs.mods.ID(),
			Block: NewBlock(
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

	fmt.Println("The proposal: ", proposal)

	cs.mods.BlockChain().Store(proposal.Block)

	cs.mods.Configuration().Propose(proposal)
	// self vote
	cs.OnPropose(proposal)
}

func (cs *consensusBase) OnPropose(proposal ProposeMsg) {
	cs.mods.Logger().Debugf("OnPropose: %v", proposal.Block)

	block := proposal.Block

	if cs.mods.Options().ShouldUseAggQC() && proposal.AggregateQC != nil {
		ok, highQC := cs.mods.Crypto().VerifyAggregateQC(*proposal.AggregateQC)
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

	defer cs.mods.Synchronizer().AdvanceView(NewSyncInfo().WithQC(block.QuorumCert()))
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
		if !ok {
			cs.mods.Logger().Info("OnPropose: Failed fetching blockhash")
		}
		cs.mods.Acceptor().Proposed(qcBlock.Command())
	}

	if !cs.mods.Acceptor().Accept(block.Command()) {
		cs.mods.Logger().Info("OnPropose: command not accepted")
		return
	}

	cs.mods.BlockChain().Store(block)

	defer func() {
		if b := cs.impl.CommitRule(block); b != nil {
			fmt.Println("Block was committed")
			cs.commit(b)
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

	leaderID := cs.mods.LeaderRotation().GetLeader(cs.lastVote) //removed +1, no difference. Added -1
	if leaderID == cs.mods.ID() {
		go cs.mods.EventLoop().AddEvent(VoteMsg{ID: cs.mods.ID(), PartialCert: pc})
		return
	} else {
		cs.mods.Logger().Info("LeaderID is NOT cs.mods.ID")
	}

	leader, ok := cs.mods.Configuration().Replica(leaderID)
	if !ok {
		cs.mods.Logger().Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(pc)
}

func (cs *consensusBase) commit(block *Block) {
	cs.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	cs.commitInner(block)
	cs.mut.Unlock()

	// prune the blockchain and handle forked blocks
	forkedBlocks := cs.mods.BlockChain().PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		cs.mods.ForkHandler().Fork(block)
	}
}

// recursive helper for commit
func (cs *consensusBase) commitInner(block *Block) {
	if cs.bExec.View() < block.View() {
		if parent, ok := cs.mods.BlockChain().Get(block.Parent()); ok {
			cs.commitInner(parent)
		}
		cs.mods.Logger().Debug("EXEC: ", block)
		cs.mods.Executor().Exec(block)
		cs.bExec = block
	}
}
