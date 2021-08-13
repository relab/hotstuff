package simplehotstuff

import "github.com/relab/hotstuff/consensus"

// SimpleHotStuff implements a simplified version of the HotStuff algorithm.
//
// Based on the simplified algorithm described in the paper
// "Formal Verification of HotStuff" by Leander Jehl.
type SimpleHotStuff struct {
	mods *consensus.Modules

	lastVote  consensus.View
	locked    *consensus.Block
	committed *consensus.Block // the last committed block
}

// New returns a new SimpleHotStuff instance.
func New() *SimpleHotStuff {
	return &SimpleHotStuff{
		lastVote:  0,
		locked:    consensus.GetGenesis(),
		committed: consensus.GetGenesis(),
	}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (hs *SimpleHotStuff) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	hs.mods = mods

	hs.mods.EventLoop().RegisterHandler(consensus.ProposeMsg{}, func(event interface{}) {
		hs.OnPropose(event.(consensus.ProposeMsg))
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (hs *SimpleHotStuff) StopVoting(view consensus.View) {
	if hs.lastVote < view {
		hs.lastVote = view
	}
}

// Propose starts a new proposal. The command is fetched from the command queue.
func (hs *SimpleHotStuff) Propose(cert consensus.SyncInfo) {
	hs.mods.Logger().Debug("Propose")
	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		qcBlock, ok := hs.mods.BlockChain().Get(qc.BlockHash())
		if !ok {
			hs.mods.Logger().Errorf("Could not find block for QC: %s", qc)
		}
		hs.mods.Acceptor().Proposed(qcBlock.Command())
	} else {
		hs.mods.Logger().Warn("Propose: no QC provided")
		return
	}

	cmd, ok := hs.mods.CommandQueue().Get(hs.mods.Synchronizer().ViewContext())
	if !ok {
		hs.mods.Logger().Debug("Propose: no command")
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

	// This broadcasts the proposal to all other replicas.
	// Note that we do not include the leader's vote in this proposal,
	// which is mentioned in the paper. Will need to confirm whether this is fine.
	hs.mods.Configuration().Propose(proposal)

	// This will generate a vote and send it to the next leader.
	hs.OnPropose(proposal)
}

// OnPropose handles a proposal.
func (hs *SimpleHotStuff) OnPropose(proposal consensus.ProposeMsg) {
	block := proposal.Block
	hs.mods.Logger().Debug("OnPropose: ", block)

	if proposal.ID != hs.mods.LeaderRotation().GetLeader(block.View()) {
		hs.mods.Logger().Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	// Rule 1: can only vote in increasing rounds
	if view := block.View(); view < hs.mods.Synchronizer().View() || view <= hs.lastVote {
		hs.mods.Logger().Info("OnPropose: block view too low")
		return
	}

	if !hs.mods.Crypto().VerifyQuorumCert(block.QuorumCert()) {
		hs.mods.Logger().Info("OnPropose: invalid QC")
		return
	}

	parent, ok := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())
	if !ok {
		hs.mods.Logger().Info("OnPropose: missing parent block: ", block.QuorumCert().BlockHash())
		return
	}

	hs.mods.Acceptor().Proposed(parent.Command())

	// Rule 2: can only vote if parent's view is greater than or equal to locked block's view.
	if parent.View() < hs.locked.View() {
		hs.mods.Logger().Info("OnPropose: parent too old")
		return
	}

	if !hs.mods.Acceptor().Accept(block.Command()) {
		hs.mods.Logger().Info("OnPropose: command not accepted")
		return
	}

	hs.lastVote = block.View()

	hs.mods.BlockChain().Store(block)

	signature, err := hs.mods.Crypto().CreatePartialCert(block)
	if err != nil {
		hs.mods.Logger().Error("OnPropose: error signing vote: ", err)
	}

	defer hs.update(block)
	defer hs.mods.Synchronizer().AdvanceView(consensus.NewSyncInfo().WithQC(block.QuorumCert()))

	leaderID := hs.mods.LeaderRotation().GetLeader(hs.lastVote + 1)
	if leaderID == hs.mods.ID() {
		go hs.mods.EventLoop().AddEvent(consensus.VoteMsg{ID: hs.mods.ID(), PartialCert: signature})
		return
	}

	leader, ok := hs.mods.Configuration().Replica(leaderID)
	if !ok {
		hs.mods.Logger().Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(signature)
}

func (hs *SimpleHotStuff) update(block *consensus.Block) {
	hs.mods.Synchronizer().UpdateHighQC(block.QuorumCert())

	// will consider if the great-grandparent of the new block can be committed.
	p, ok := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())
	if !ok {
		return
	}

	gp, ok := hs.mods.BlockChain().Get(p.QuorumCert().BlockHash())
	if ok && gp.View() > hs.locked.View() {
		hs.locked = gp
		hs.mods.Logger().Debug("Locked: ", gp)
	} else if !ok {
		return
	}

	ggp, ok := hs.mods.BlockChain().Get(gp.QuorumCert().BlockHash())
	// we commit the great-grandparent of the block if its grandchild is certified,
	// which we already know is true because the new block contains the grandchild's certificate,
	// and if the great-grandparent's view + 2 equals the grandchild's view.
	if ok && ggp.View()+2 == p.View() {
		hs.commit(ggp)
	}
}

func (hs *SimpleHotStuff) commit(block *consensus.Block) {
	if hs.committed.View() < block.View() {
		// assuming that it is okay to commit the ancestors of the block too.
		if parent, ok := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash()); ok {
			hs.commit(parent)
		}
		hs.mods.Logger().Debug("EXEC: ", block)
		hs.mods.Executor().Exec(block.Command())
		hs.committed = block
	}
}
