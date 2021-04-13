package chainedhotstuff

import "github.com/relab/hotstuff"

type chainedhotstuff struct {
	mod *hotstuff.HotStuff

	// protocol variables

	lastVote hotstuff.View   // the last view that the replica voted in
	bLock    *hotstuff.Block // the currently locked block
	bExec    *hotstuff.Block // the last committed block

	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert // verified votes that could become a QC
}

// New returns a new chainedhotstuff instance.
func New() hotstuff.Consensus {
	hs := &chainedhotstuff{}
	hs.verifiedVotes = make(map[hotstuff.Hash][]hotstuff.PartialCert)
	hs.bLock = hotstuff.GetGenesis()
	hs.bExec = hotstuff.GetGenesis()
	return hs
}

func (hs *chainedhotstuff) InitModule(mod *hotstuff.HotStuff) {
	hs.mod = mod
}

// LastVote returns the view in which the replica last voted.
func (hs *chainedhotstuff) LastVote() hotstuff.View {
	return hs.lastVote
}

// IncreaseLastVotedView ensures that no voting happens in a view earlier than `view`.
func (hs *chainedhotstuff) IncreaseLastVotedView(view hotstuff.View) {
	hs.lastVote++
}

func (hs *chainedhotstuff) commit(block *hotstuff.Block) {
	if hs.bExec.View() < block.View() {
		if parent, ok := hs.mod.BlockChain().Get(block.Parent()); ok {
			hs.commit(parent)
		}
		hs.mod.Logger().Debug("EXEC: ", block)
		hs.mod.Executor().Exec(block.Command())
		hs.bExec = block
	}
}

func (hs *chainedhotstuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.mod.BlockChain().Get(qc.BlockHash())
}

func (hs *chainedhotstuff) update(block *hotstuff.Block) {
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
func (hs *chainedhotstuff) Propose() {
	hs.mod.Logger().Debug("Propose")

	cmd, ok := hs.mod.CommandQueue().Get(hs.mod.ViewSynchronizer().ViewContext())
	if !ok {
		return
	}
	block := hotstuff.NewBlock(
		hs.mod.ViewSynchronizer().LeafBlock().Hash(),
		hs.mod.ViewSynchronizer().HighQC(),
		cmd,
		hs.mod.ViewSynchronizer().View(),
		hs.mod.ID(),
	)
	hs.mod.BlockChain().Store(block)

	hs.mod.Manager().Propose(block)
	// self vote
	hs.OnPropose(hotstuff.ProposeMsg{ID: hs.mod.ID(), Block: block})
}

// OnPropose handles an incoming proposal
func (hs *chainedhotstuff) OnPropose(proposal hotstuff.ProposeMsg) {
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

	qcBlock, haveQCBlock := hs.mod.BlockChain().Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.mod.Logger().Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		b := block
		ok := true
		for ok && b.View() > hs.bLock.View() {
			b, ok = hs.mod.BlockChain().Get(b.Parent())
		}
		if ok && b.Hash() == hs.bLock.Hash() {
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
		hs.mod.ViewSynchronizer().AdvanceView(hotstuff.SyncInfoWithQC(block.QuorumCert()))
	}

	leaderID := hs.mod.LeaderRotation().GetLeader(hs.lastVote + 1)
	if leaderID == hs.mod.ID() {
		finish()
		hs.OnVote(hotstuff.VoteMsg{ID: hs.mod.ID(), PartialCert: pc})
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

// OnVote handles an incoming vote
func (hs *chainedhotstuff) OnVote(vote hotstuff.VoteMsg) {
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range hs.verifiedVotes {
			if block, ok := hs.mod.BlockChain().LocalGet(k); ok {
				if block.View() <= hs.mod.ViewSynchronizer().LeafBlock().View() {
					delete(hs.verifiedVotes, k)
				}
			} else {
				delete(hs.verifiedVotes, k)
			}
		}
	}()

	cert := vote.PartialCert
	hs.mod.Logger().Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *hotstuff.Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = hs.mod.BlockChain().LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			hs.mod.Logger().Infof("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			hs.mod.EventLoop().AwaitEvent(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = hs.mod.BlockChain().Get(cert.BlockHash())
		if !ok {
			hs.mod.Logger().Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= hs.mod.ViewSynchronizer().LeafBlock().View() {
		// too old
		return
	}

	if !hs.mod.Crypto().VerifyPartialCert(cert) {
		hs.mod.Logger().Info("OnVote: Vote could not be verified!")
		return
	}

	votes := hs.verifiedVotes[cert.BlockHash()]
	votes = append(votes, cert)
	hs.verifiedVotes[cert.BlockHash()] = votes

	if len(votes) < hs.mod.Manager().QuorumSize() {
		return
	}

	qc, err := hs.mod.Crypto().CreateQuorumCert(block, votes)
	if err != nil {
		hs.mod.Logger().Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(hs.verifiedVotes, cert.BlockHash())

	// signal the synchronizer
	hs.mod.ViewSynchronizer().AdvanceView(hotstuff.SyncInfoWithQC(qc))
}

var _ hotstuff.Consensus = (*chainedhotstuff)(nil)
