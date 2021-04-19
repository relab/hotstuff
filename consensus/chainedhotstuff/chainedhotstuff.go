package chainedhotstuff

import "github.com/relab/hotstuff"

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	mod *hotstuff.HotStuff

	// protocol variables

	lastVote hotstuff.View   // the last view that the replica voted in
	bLock    *hotstuff.Block // the currently locked block
	bExec    *hotstuff.Block // the last committed block
}

// New returns a new chainedhotstuff instance.
func New() *ChainedHotStuff {
	hs := &ChainedHotStuff{}
	hs.bLock = hotstuff.GetGenesis()
	hs.bExec = hotstuff.GetGenesis()
	return hs
}

// InitModule gives ChainedHotstuff a pointer to the other modules.
func (hs *ChainedHotStuff) InitModule(mod *hotstuff.HotStuff, _ *hotstuff.ConfigBuilder) {
	hs.mod = mod
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (hs *ChainedHotStuff) StopVoting(view hotstuff.View) {
	if hs.lastVote < view {
		hs.lastVote = view
	}
}

func (hs *ChainedHotStuff) commit(block *hotstuff.Block) {
	if hs.bExec.View() < block.View() {
		if parent, ok := hs.mod.BlockChain().Get(block.Parent()); ok {
			hs.commit(parent)
		}
		hs.mod.Logger().Debug("EXEC: ", block)
		hs.mod.Executor().Exec(block.Command())
		hs.bExec = block
	}
}

func (hs *ChainedHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.mod.BlockChain().Get(qc.BlockHash())
}

func (hs *ChainedHotStuff) update(block *hotstuff.Block) {
	hs.mod.ViewSynchronizer().UpdateHighQC(block.QuorumCert())

	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return
	}

	hs.mod.Logger().Debug("PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return
	}

	if block2.View() > hs.bLock.View() {
		hs.mod.Logger().Debug("COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.mod.Logger().Debug("DECIDE: ", block3)
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
func (hs *ChainedHotStuff) Propose(cert hotstuff.SyncInfo) {
	hs.mod.Logger().Debug("Propose")

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		qcBlock, ok := hs.mod.BlockChain().Get(qc.BlockHash())
		if !ok {
			hs.mod.Logger().Error("Could not find block for QC: %s", qc)
			return
		}
		hs.mod.Acceptor().Proposed(qcBlock.Command())
	} else {
		hs.mod.Logger().Warn("Propose: no QC provided.")
	}

	cmd, ok := hs.mod.CommandQueue().Get(hs.mod.ViewSynchronizer().ViewContext())
	if !ok {
		return
	}
	block := hotstuff.NewBlock(
		hs.mod.ViewSynchronizer().LeafBlock().Hash(),
		qc,
		cmd,
		hs.mod.ViewSynchronizer().View(),
		hs.mod.ID(),
	)
	hs.mod.BlockChain().Store(block)

	proposal := hotstuff.ProposeMsg{ID: hs.mod.ID(), Block: block}
	hs.mod.Manager().Propose(proposal)
	// self vote
	hs.OnPropose(proposal)
}

// OnPropose handles an incoming proposal
func (hs *ChainedHotStuff) OnPropose(proposal hotstuff.ProposeMsg) {
	block := proposal.Block
	hs.mod.Logger().Debug("OnPropose: ", block)

	if proposal.ID != hs.mod.LeaderRotation().GetLeader(block.View()) {
		hs.mod.Logger().Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if block.View() < hs.mod.ViewSynchronizer().View() {
		hs.mod.Logger().Info("OnPropose: block view was less than our view")
		return
	}

	if !hs.mod.Crypto().VerifyQuorumCert(block.QuorumCert()) {
		hs.mod.Logger().Info("OnPropose: invalid QC")
		return
	}

	qcBlock, haveQCBlock := hs.mod.BlockChain().Get(block.QuorumCert().BlockHash())
	// Tell the acceptor that the QC's block was proposed successfully.
	hs.mod.Acceptor().Proposed(qcBlock.Command())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.mod.Logger().Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		if hs.mod.BlockChain().Extends(block, hs.bLock) {
			safe = true
		} else {
			hs.mod.Logger().Debug("OnPropose: safety condition failed")
		}
	}

	if !safe {
		hs.mod.Logger().Info("OnPropose: block not safe")
		return
	}

	if !hs.mod.Acceptor().Accept(block.Command()) {
		hs.mod.Logger().Info("OnPropose: command not accepted")
		return
	}

	hs.mod.BlockChain().Store(block)

	pc, err := hs.mod.Crypto().CreatePartialCert(block)
	if err != nil {
		hs.mod.Logger().Error("OnPropose: failed to sign vote: ", err)
		return
	}

	hs.lastVote = block.View()

	finish := func() {
		hs.update(block)
		hs.mod.ViewSynchronizer().AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))
	}

	leaderID := hs.mod.LeaderRotation().GetLeader(hs.lastVote + 1)
	if leaderID == hs.mod.ID() {
		finish()
		hs.mod.VotingMachine().OnVote(hotstuff.VoteMsg{ID: hs.mod.ID(), PartialCert: pc})
		return
	}

	leader, ok := hs.mod.Manager().Replica(leaderID)
	if !ok {
		hs.mod.Logger().Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(pc)
	finish()
}

var _ hotstuff.Consensus = (*ChainedHotStuff)(nil)
