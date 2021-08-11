// Package chainedhotstuff implements the pipelined three-chain version of the HotStuff protocol.
package chainedhotstuff

import (
	"github.com/relab/hotstuff/consensus"
)

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	mods *consensus.Modules

	// protocol variables

	lastVote consensus.View   // the last view that the replica voted in
	bLock    *consensus.Block // the currently locked block
	bExec    *consensus.Block // the last committed block
}

// New returns a new chainedhotstuff instance.
func New() *ChainedHotStuff {
	hs := &ChainedHotStuff{}
	hs.bLock = consensus.GetGenesis()
	hs.bExec = consensus.GetGenesis()
	return hs
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (hs *ChainedHotStuff) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	hs.mods = mods
	hs.mods.EventLoop().RegisterHandler(consensus.ProposeMsg{}, func(event interface{}) {
		proposal := event.(consensus.ProposeMsg)
		hs.OnPropose(proposal)
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (hs *ChainedHotStuff) StopVoting(view consensus.View) {
	if hs.lastVote < view {
		hs.lastVote = view
	}
}

func (hs *ChainedHotStuff) commit(block *consensus.Block) {
	if hs.bExec.View() < block.View() {
		if parent, ok := hs.mods.BlockChain().Get(block.Parent()); ok {
			hs.commit(parent)
		}
		hs.mods.Logger().Debug("EXEC: ", block)
		hs.mods.Executor().Exec(block.Command())
		hs.bExec = block
	}
}

func (hs *ChainedHotStuff) qcRef(qc consensus.QuorumCert) (*consensus.Block, bool) {
	if (consensus.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.mods.BlockChain().Get(qc.BlockHash())
}

func (hs *ChainedHotStuff) update(block *consensus.Block) {
	hs.mods.Synchronizer().UpdateHighQC(block.QuorumCert())

	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return
	}

	hs.mods.Logger().Debug("PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return
	}

	if block2.View() > hs.bLock.View() {
		hs.mods.Logger().Debug("COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.mods.Logger().Debug("DECIDE: ", block3)
		hs.commit(block3)
		// NOTE: we now update bExec in commit, instead of doing it here. Updating bExec here can be problematic,
		// because it can lead to a block being executed twice. Let's consider a view where the leader's proposal is not
		// accepted by the other replicas. In that case, the leader will have called update(), which leads to a block
		// being executed. Let's call the view that failed 'v'. In the next view, v+1, the new leader will select the
		// highest QC it knows. This should be the QC from view v-1, which references the block from v-2. When this is
		// proposed to the old leader, it will again call update(), but this time it will not update bLock. It will,
		// however, find a three chain and call execute. The block that has a three-chain will have a lower view (v-4)
		// than bExec (v-3) at that point. commit() handles this correctly, and will not execute the block twice, but
		// because we used to update bExec right here, the next view would cause the block from view v-3 to be executed
		// again. By updating bExec within commit() instead, we solve this problem.
		//
		// The leader of view v: After view v
		//         bExec   bLock
		//           |       |
		//           v       v
		// ->[v-4]->[v-3]->[v-2]->[v-1]->[v]
		//
		// After view v+1 (assuming that we update bExec on the line below this comment):
		//   bExec         bLock
		//     |             |
		//     v             v
		// ->[v-4]->[v-3]->[v-2]->[v-1]->[v+1]
	}
}

// Propose proposes the given command
func (hs *ChainedHotStuff) Propose(cert consensus.SyncInfo) {
	hs.mods.Logger().Debug("Propose")

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		qcBlock, ok := hs.mods.BlockChain().Get(qc.BlockHash())
		if !ok {
			hs.mods.Logger().Error("Could not find block for QC: %s", qc)
			return
		}
		hs.mods.Acceptor().Proposed(qcBlock.Command())
	} else {
		hs.mods.Logger().Warn("Propose: no QC provided.")
	}

	cmd, ok := hs.mods.CommandQueue().Get(hs.mods.Synchronizer().ViewContext())
	if !ok {
		return
	}
	block := consensus.NewBlock(
		hs.mods.Synchronizer().LeafBlock().Hash(),
		qc,
		cmd,
		hs.mods.Synchronizer().View(),
		hs.mods.ID(),
	)
	hs.mods.BlockChain().Store(block)

	proposal := consensus.ProposeMsg{ID: hs.mods.ID(), Block: block}
	hs.mods.Configuration().Propose(proposal)
	// self vote
	hs.OnPropose(proposal)
}

// OnPropose handles an incoming proposal
func (hs *ChainedHotStuff) OnPropose(proposal consensus.ProposeMsg) {
	block := proposal.Block
	hs.mods.Logger().Debug("OnPropose: ", block)

	if proposal.ID != hs.mods.LeaderRotation().GetLeader(block.View()) {
		hs.mods.Logger().Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if view := block.View(); view < hs.mods.Synchronizer().View() || view <= hs.lastVote {
		hs.mods.Logger().Info("OnPropose: block view too low")
		return
	}

	if !hs.mods.Crypto().VerifyQuorumCert(block.QuorumCert()) {
		hs.mods.Logger().Info("OnPropose: invalid QC")
		return
	}

	qcBlock, haveQCBlock := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.mods.Logger().Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		if hs.mods.BlockChain().Extends(block, hs.bLock) {
			safe = true
		} else {
			hs.mods.Logger().Debug("OnPropose: safety condition failed")
		}
	}

	if !safe {
		hs.mods.Logger().Info("OnPropose: block not safe")
		return
	}

	if haveQCBlock {
		// Tell the acceptor that the QC's block was proposed successfully.
		hs.mods.Acceptor().Proposed(qcBlock.Command())
	}

	if !hs.mods.Acceptor().Accept(block.Command()) {
		hs.mods.Logger().Info("OnPropose: command not accepted")
		return
	}

	hs.mods.BlockChain().Store(block)

	pc, err := hs.mods.Crypto().CreatePartialCert(block)
	if err != nil {
		hs.mods.Logger().Error("OnPropose: failed to sign vote: ", err)
		return
	}

	hs.lastVote = block.View()

	finish := func() {
		hs.update(block)
		hs.mods.Synchronizer().AdvanceView(consensus.NewSyncInfo().WithQC(block.QuorumCert()))
	}

	leaderID := hs.mods.LeaderRotation().GetLeader(hs.lastVote + 1)
	if leaderID == hs.mods.ID() {
		finish()
		go hs.mods.EventLoop().AddEvent(consensus.VoteMsg{ID: hs.mods.ID(), PartialCert: pc})
		return
	}

	leader, ok := hs.mods.Configuration().Replica(leaderID)
	if !ok {
		hs.mods.Logger().Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(pc)
	finish()
}

var _ consensus.Consensus = (*ChainedHotStuff)(nil)
