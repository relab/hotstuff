package fasthotstuff

import "github.com/relab/hotstuff"

// FastHotStuff is an implementation of the Fast-HotStuff protocol.
type FastHotStuff struct {
	mod *hotstuff.HotStuff

	bExec    *hotstuff.Block
	lastVote hotstuff.View
}

// New returns a new FastHotStuff instance.
func New() *FastHotStuff {
	return &FastHotStuff{
		bExec:    hotstuff.GetGenesis(),
		lastVote: 0,
	}
}

// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
// settings using the ConfigBuilder.
func (fhs *FastHotStuff) InitModule(hs *hotstuff.HotStuff, cfg *hotstuff.ConfigBuilder) {
	fhs.mod = hs
	cfg.SetShouldUseAggQC()
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (fhs *FastHotStuff) StopVoting(view hotstuff.View) {
	if fhs.lastVote < view {
		fhs.lastVote = view
	}
}

// Propose starts a new proposal. The command is fetched from the command queue.
func (fhs *FastHotStuff) Propose(cert hotstuff.SyncInfo) {
	fhs.mod.Logger().Debug("Propose")

	proposal := hotstuff.ProposeMsg{ID: fhs.mod.ID()}

	if aggQC, ok := cert.AggQC(); ok {
		proposal.AggregateQC = &aggQC
	}

	qc, ok := cert.QC()
	if !ok {
		fhs.mod.Logger().Warnf("Propose was called with no QC!")
		return
	}

	cmd, ok := fhs.mod.CommandQueue().Get(fhs.mod.ViewSynchronizer().ViewContext())
	if !ok {
		return
	}

	proposal.Block = hotstuff.NewBlock(
		fhs.mod.ViewSynchronizer().LeafBlock().Hash(),
		qc,
		cmd,
		fhs.mod.ViewSynchronizer().View(),
		fhs.mod.ID(),
	)

	fhs.mod.BlockChain().Store(proposal.Block)

	fhs.mod.Manager().Propose(proposal)
	fhs.OnPropose(proposal)
}

func (fhs *FastHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.mod.BlockChain().Get(qc.BlockHash())
}

func (fhs *FastHotStuff) execute(block *hotstuff.Block) {
	if fhs.bExec.View() < block.View() {
		if parent, ok := fhs.mod.BlockChain().Get(block.Parent()); ok {
			fhs.execute(parent)
		}
		fhs.mod.Logger().Debug("EXEC: ", block)
		fhs.mod.Executor().Exec(block.Command())
		fhs.bExec = block
	}
}

func (fhs *FastHotStuff) update(block *hotstuff.Block) {
	fhs.mod.ViewSynchronizer().UpdateHighQC(block.QuorumCert())
	fhs.mod.Logger().Debug("PREPARE: ", block)

	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return
	}
	fhs.mod.Logger().Debug("PRECOMMIT: ", parent)

	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return
	}
	if block.Parent() == parent.Hash() && block.View() == parent.View()+1 &&
		parent.Parent() == grandparent.Hash() && parent.View() == grandparent.View()+1 {
		fhs.mod.Logger().Debug("COMMIT: ", grandparent)
		fhs.execute(grandparent)
	}
}

// OnPropose handles an incoming proposal.
// A leader should call this method on itself.
func (fhs *FastHotStuff) OnPropose(proposal hotstuff.ProposeMsg) {
	safe := false
	block := proposal.Block

	if proposal.AggregateQC == nil {
		safe = fhs.mod.Crypto().VerifyQuorumCert(block.QuorumCert()) &&
			block.View() >= fhs.mod.ViewSynchronizer().View() &&
			block.View() == block.QuorumCert().View()+1
	} else {
		// If we get an AggregateQC, we need to verify the AggregateQC, and the highQC it contains.
		// Then, we must check that the proposed block extends the highQC.block.
		ok, highQC := fhs.mod.Crypto().VerifyAggregateQC(*proposal.AggregateQC)
		if ok && fhs.mod.Crypto().VerifyQuorumCert(highQC) {
			hqcBlock, ok := fhs.mod.BlockChain().Get(highQC.BlockHash())
			if ok && fhs.mod.BlockChain().Extends(block, hqcBlock) {
				safe = true
				// create a new block containing the QC from the aggregateQC
				block = hotstuff.NewBlock(block.Parent(), highQC, block.Command(), block.View(), block.Proposer())
			}
		}
	}

	defer fhs.update(block)

	if !safe {
		fhs.mod.Logger().Info("OnPropose: block not safe")
		return
	}

	defer fhs.mod.ViewSynchronizer().AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))

	if !fhs.mod.Acceptor().Accept(block.Command()) {
		fhs.mod.Logger().Info("OnPropose: command not accepted")
		return
	}

	fhs.mod.BlockChain().Store(block)

	if fhs.lastVote >= block.View() {
		// already voted, or StopVoting was called for this view.
		return
	}

	vote, err := fhs.mod.Crypto().CreatePartialCert(block)
	if err != nil {
		fhs.mod.Logger().Error("OnPropose: failed to sign block: ", err)
		return
	}

	fhs.lastVote = block.View()

	leaderID := fhs.mod.LeaderRotation().GetLeader(block.View() + 1)
	if leaderID == fhs.mod.ID() {
		fhs.mod.VotingMachine().OnVote(hotstuff.VoteMsg{ID: fhs.mod.ID(), PartialCert: vote})
		return
	}

	leader, ok := fhs.mod.Manager().Replica(leaderID)
	if !ok {
		fhs.mod.Logger().Warn("Leader with ID %d was not found", leaderID)
		return
	}

	leader.Vote(vote)
}

var _ hotstuff.Consensus = (*FastHotStuff)(nil)
