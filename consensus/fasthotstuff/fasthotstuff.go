// Package fasthotstuff implements the two-chain Fast-HotStuff protocol.
package fasthotstuff

import (
	"github.com/relab/hotstuff/consensus"
)

// FastHotStuff is an implementation of the Fast-HotStuff protocol.
type FastHotStuff struct {
	mods *consensus.Modules

	bExec    *consensus.Block
	lastVote consensus.View
}

// New returns a new FastHotStuff instance.
func New() *FastHotStuff {
	return &FastHotStuff{
		bExec:    consensus.GetGenesis(),
		lastVote: 0,
	}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (fhs *FastHotStuff) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	fhs.mods = mods
	opts.SetShouldUseAggQC()
	fhs.mods.EventLoop().RegisterHandler(consensus.ProposeMsg{}, func(event interface{}) {
		proposal := event.(consensus.ProposeMsg)
		fhs.OnPropose(proposal)
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (fhs *FastHotStuff) StopVoting(view consensus.View) {
	if fhs.lastVote < view {
		fhs.lastVote = view
	}
}

// Propose starts a new proposal. The command is fetched from the command queue.
func (fhs *FastHotStuff) Propose(cert consensus.SyncInfo) {
	fhs.mods.Logger().Debug("Propose")

	proposal := consensus.ProposeMsg{ID: fhs.mods.ID()}

	if aggQC, ok := cert.AggQC(); ok {
		proposal.AggregateQC = &aggQC
	}

	qc, ok := cert.QC()
	if ok {
		qcBlock, ok := fhs.mods.BlockChain().Get(qc.BlockHash())
		if !ok {
			fhs.mods.Logger().Errorf("Could not get block for QC: %s", qc)
			return
		}
		fhs.mods.Acceptor().Proposed(qcBlock.Command())
	} else {
		fhs.mods.Logger().Warnf("Propose was called with no QC!")
		return
	}

	cmd, ok := fhs.mods.CommandQueue().Get(fhs.mods.Synchronizer().ViewContext())
	if !ok {
		return
	}

	proposal.Block = consensus.NewBlock(
		fhs.mods.Synchronizer().LeafBlock().Hash(),
		qc,
		cmd,
		fhs.mods.Synchronizer().View(),
		fhs.mods.ID(),
	)

	fhs.mods.BlockChain().Store(proposal.Block)

	fhs.mods.Configuration().Propose(proposal)
	fhs.OnPropose(proposal)
}

func (fhs *FastHotStuff) qcRef(qc consensus.QuorumCert) (*consensus.Block, bool) {
	if (consensus.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.mods.BlockChain().Get(qc.BlockHash())
}

func (fhs *FastHotStuff) execute(block *consensus.Block) {
	if fhs.bExec.View() < block.View() {
		if parent, ok := fhs.mods.BlockChain().Get(block.Parent()); ok {
			fhs.execute(parent)
		}
		fhs.mods.Logger().Debug("EXEC: ", block)
		fhs.mods.Executor().Exec(block.Command())
		fhs.bExec = block
	}
}

func (fhs *FastHotStuff) update(block *consensus.Block) {
	fhs.mods.Synchronizer().UpdateHighQC(block.QuorumCert())
	fhs.mods.Logger().Debug("PREPARE: ", block)

	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return
	}
	fhs.mods.Logger().Debug("PRECOMMIT: ", parent)

	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return
	}
	if block.Parent() == parent.Hash() && block.View() == parent.View()+1 &&
		parent.Parent() == grandparent.Hash() && parent.View() == grandparent.View()+1 {
		fhs.mods.Logger().Debug("COMMIT: ", grandparent)
		fhs.execute(grandparent)
	}
}

// OnPropose handles an incoming proposal.
// A leader should call this method on itself.
func (fhs *FastHotStuff) OnPropose(proposal consensus.ProposeMsg) {
	fhs.mods.Logger().Debugf("OnPropose: %s", proposal.Block)

	var (
		safe     = false
		block    = proposal.Block
		hqcBlock *consensus.Block
		ok       bool
	)

	if proposal.ID != fhs.mods.LeaderRotation().GetLeader(block.View()) {
		fhs.mods.Logger().Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if proposal.AggregateQC == nil {
		safe = fhs.mods.Crypto().VerifyQuorumCert(block.QuorumCert()) &&
			block.View() >= fhs.mods.Synchronizer().View() &&
			block.View() == block.QuorumCert().View()+1
		hqcBlock, ok = fhs.mods.BlockChain().Get(block.QuorumCert().BlockHash())
		if !ok {
			fhs.mods.Logger().Warn("Missing block for QC: %s", block.QuorumCert())
			return
		}
	} else {
		// If we get an AggregateQC, we need to verify the AggregateQC, and the highQC it contains.
		// Then, we must check that the proposed block extends the highQC.block.
		ok, highQC := fhs.mods.Crypto().VerifyAggregateQC(*proposal.AggregateQC)
		if ok && fhs.mods.Crypto().VerifyQuorumCert(highQC) {
			hqcBlock, ok = fhs.mods.BlockChain().Get(highQC.BlockHash())
			if ok && fhs.mods.BlockChain().Extends(block, hqcBlock) {
				safe = true
				// create a new block containing the QC from the aggregateQC
				block = consensus.NewBlock(block.Parent(), highQC, block.Command(), block.View(), block.Proposer())
			}
		}
	}

	defer fhs.update(block)

	if !safe {
		fhs.mods.Logger().Info("OnPropose: block not safe")
		return
	}

	fhs.mods.Acceptor().Proposed(hqcBlock.Command())

	fhs.mods.BlockChain().Store(block)
	defer fhs.mods.Synchronizer().AdvanceView(consensus.NewSyncInfo().WithQC(block.QuorumCert()))

	if fhs.lastVote >= block.View() {
		// already voted, or StopVoting was called for this view.
		return
	}

	if !fhs.mods.Acceptor().Accept(block.Command()) {
		fhs.mods.Logger().Info("OnPropose: command not accepted")
		return
	}

	vote, err := fhs.mods.Crypto().CreatePartialCert(block)
	if err != nil {
		fhs.mods.Logger().Error("OnPropose: failed to sign block: ", err)
		return
	}

	fhs.lastVote = block.View()

	leaderID := fhs.mods.LeaderRotation().GetLeader(block.View() + 1)
	if leaderID == fhs.mods.ID() {
		go fhs.mods.EventLoop().AddEvent(consensus.VoteMsg{ID: fhs.mods.ID(), PartialCert: vote})
		return
	}

	leader, ok := fhs.mods.Configuration().Replica(leaderID)
	if !ok {
		fhs.mods.Logger().Warn("Leader with ID %d was not found", leaderID)
		return
	}

	leader.Vote(vote)
}

var _ consensus.Consensus = (*FastHotStuff)(nil)
