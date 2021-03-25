package chainedhotstuff

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/logging"
)

var logger = logging.GetLogger()

type chainedhotstuff struct {
	mod *hotstuff.HotStuff

	// protocol variables

	lastVote hotstuff.View       // the last view that the replica voted in
	bLock    *hotstuff.Block     // the currently locked block
	bExec    *hotstuff.Block     // the last committed block
	bLeaf    *hotstuff.Block     // the last proposed block
	highQC   hotstuff.QuorumCert // the highest qc known to this replica

	fetchCancel context.CancelFunc

	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert   // verified votes that could become a QC
	pendingVotes  map[hotstuff.Hash][]hotstuff.VoteMsg       // unverified votes that are waiting for a Block
	newView       map[hotstuff.View]map[hotstuff.ID]struct{} // the set of replicas who have sent a newView message per view
}

// New returns a new chainedhotstuff instance.
func New() hotstuff.Consensus {
	hs := &chainedhotstuff{}
	hs.verifiedVotes = make(map[hotstuff.Hash][]hotstuff.PartialCert)
	hs.pendingVotes = make(map[hotstuff.Hash][]hotstuff.VoteMsg)
	hs.newView = make(map[hotstuff.View]map[hotstuff.ID]struct{})
	hs.fetchCancel = func() {}
	hs.bLock = hotstuff.GetGenesis()
	hs.bExec = hotstuff.GetGenesis()
	hs.bLeaf = hotstuff.GetGenesis()
	return hs
}

func (hs *chainedhotstuff) InitModule(mod *hotstuff.HotStuff) {
	hs.mod = mod

	var err error
	hs.highQC, err = hs.mod.Signer().CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		logger.Panicf("Failed to create QC for genesis block!")
	}
}

// LastVote returns the view in which the replica last voted.
func (hs *chainedhotstuff) LastVote() hotstuff.View {
	return hs.lastVote
}

// IncreaseLastVotedView ensures that no voting happens in a view earlier than `view`.
func (hs *chainedhotstuff) IncreaseLastVotedView(view hotstuff.View) {
	hs.lastVote++
}

// HighQC returns the highest QC known to the replica
func (hs *chainedhotstuff) HighQC() hotstuff.QuorumCert {

	return hs.highQC
}

// Leaf returns the last proposed block
func (hs *chainedhotstuff) Leaf() *hotstuff.Block {
	return hs.bLeaf
}

func (hs *chainedhotstuff) CreateDummy() {
	dummy := hotstuff.NewBlock(hs.bLeaf.Hash(), nil, hotstuff.Command(""), hs.bLeaf.View()+1, hs.mod.ID())
	hs.mod.BlockChain().Store(dummy)
	hs.bLeaf = dummy
}

// UpdateHighQC updates HighQC if the given qc is higher than the old HighQC.
func (hs *chainedhotstuff) UpdateHighQC(qc hotstuff.QuorumCert) {
	hs.updateHighQC(qc)
}

// updateHighQC differs from the exported version because it does not lock the mutex.
func (hs *chainedhotstuff) updateHighQC(qc hotstuff.QuorumCert) {
	logger.Debugf("updateHighQC: %v", qc)
	if !hs.mod.Verifier().VerifyQuorumCert(qc) {
		logger.Info("updateHighQC: QC could not be verified!")
		return
	}

	newBlock, ok := hs.mod.BlockChain().Get(qc.BlockHash())
	if !ok {
		logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	oldBlock, ok := hs.mod.BlockChain().Get(hs.highQC.BlockHash())
	if !ok {
		logger.Panic("Block from the old highQC missing from chain")
	}

	if newBlock.View() > oldBlock.View() {
		hs.highQC = qc
		hs.bLeaf = newBlock
	}
}

func (hs *chainedhotstuff) commit(block *hotstuff.Block) {
	if hs.bExec.View() < block.View() {
		if parent, ok := hs.mod.BlockChain().Get(block.Parent()); ok {
			hs.commit(parent)
		}
		if block.QuorumCert() == nil {
			// don't execute dummy nodes
			return
		}
		logger.Debug("EXEC: ", block)
		hs.mod.Executor().Exec(block.Command())
	}
}

func (hs *chainedhotstuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if qc == nil {
		return nil, false
	}
	return hs.mod.BlockChain().Get(qc.BlockHash())
}

func (hs *chainedhotstuff) update(block *hotstuff.Block) {
	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return
	}

	logger.Debug("PRE_COMMIT: ", block1)
	hs.updateHighQC(block.QuorumCert())

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return
	}

	if block2.View() > hs.bLock.View() {
		logger.Debug("COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		logger.Debug("DECIDE: ", block3)
		hs.commit(block3)
		hs.bExec = block3
	}
}

// Propose proposes the given command
func (hs *chainedhotstuff) Propose() {
	logger.Debug("Propose")

	cmd := hs.mod.CommandQueue().GetCommand()
	// TODO: Should probably use channels/contexts here instead such that
	// a proposal can be made a little later if a new command is added to the queue.
	// Alternatively, we could let the pacemaker know when commands arrive, so that it
	// can rall Propose() again.
	if cmd == nil {
		// return
		cmd = new(hotstuff.Command)
	}
	block := hotstuff.NewBlock(hs.bLeaf.Hash(), hs.highQC, *cmd, hs.mod.ViewSynchronizer().View(), hs.mod.ID())
	hs.mod.BlockChain().Store(block)

	hs.mod.Config().Propose(block)
	// self vote
	hs.OnPropose(hotstuff.ProposeMsg{ID: hs.mod.ID(), Block: block})
}

// OnPropose handles an incoming proposal
func (hs *chainedhotstuff) OnPropose(proposal hotstuff.ProposeMsg) {
	block := proposal.Block
	logger.Debug("OnPropose: ", block)

	if proposal.ID != hs.mod.LeaderRotation().GetLeader(block.View()) {
		logger.Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if block.View() < hs.mod.ViewSynchronizer().View() {
		logger.Info("OnPropose: block view was less than our view")
		return
	}

	qcBlock, haveQCBlock := hs.mod.BlockChain().Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		logger.Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		b := block
		ok := true
		for ok && b.View() > hs.bLock.View() {
			b, ok = hs.mod.BlockChain().Get(b.Parent())
		}
		if ok && b.Hash() == hs.bLock.Hash() {
			safe = true
		} else {
			logger.Debug("OnPropose: safety condition failed")
		}
	}

	if !safe {
		logger.Info("OnPropose: block not safe")
		return
	}

	if !hs.mod.Acceptor().Accept(block.Command()) {
		logger.Info("OnPropose: command not accepted")
		return
	}

	// cancel the last fetch
	hs.fetchCancel()
	hs.mod.BlockChain().Store(block)

	pc, err := hs.mod.Signer().CreatePartialCert(block)
	if err != nil {
		logger.Error("OnPropose: failed to sign vote: ", err)
		return
	}

	hs.lastVote = block.View()

	finish := func() {
		hs.update(block)
		hs.deliver(block)
		hs.pendingVotes = make(map[hotstuff.Hash][]hotstuff.VoteMsg)
		hs.mod.ViewSynchronizer().AdvanceView(hotstuff.SyncInfo{QC: block.QuorumCert()})
	}

	leaderID := hs.mod.LeaderRotation().GetLeader(hs.lastVote + 1)
	if leaderID == hs.mod.ID() {
		finish()
		hs.OnVote(hotstuff.VoteMsg{ID: hs.mod.ID(), PartialCert: pc})
		return
	}

	leader, ok := hs.mod.Config().Replica(leaderID)
	if !ok {
		logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(pc)
	finish()
}

func (hs *chainedhotstuff) fetchBlockForVote(voteMsg hotstuff.VoteMsg) {
	vote := voteMsg.PartialCert
	votes, ok := hs.pendingVotes[vote.BlockHash()]
	votes = append(votes, voteMsg)
	hs.pendingVotes[vote.BlockHash()] = votes

	if ok {
		// another vote initiated fetching
		return
	}

	var ctx context.Context
	ctx, hs.fetchCancel = context.WithCancel(context.Background())

	hs.mod.Config().Fetch(ctx, vote.BlockHash())
}

// OnVote handles an incoming vote
func (hs *chainedhotstuff) OnVote(vote hotstuff.VoteMsg) {
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range hs.verifiedVotes {
			if block, ok := hs.mod.BlockChain().Get(k); ok {
				if block.View() <= hs.bLeaf.View() {
					delete(hs.verifiedVotes, k)
				}
			} else {
				delete(hs.verifiedVotes, k)
			}
		}
	}()

	cert := vote.PartialCert

	block, ok := hs.mod.BlockChain().Get(cert.BlockHash())
	if !ok {
		logger.Debugf("Could not find block for vote: %.8s. Attempting to fetch.", cert.BlockHash())
		hs.fetchBlockForVote(vote)
		return
	}

	if block.View() <= hs.bLeaf.View() {
		// too old
		return
	}

	if !hs.mod.Verifier().VerifyPartialCert(cert) {
		logger.Info("OnVote: Vote could not be verified!")
		return
	}

	logger.Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	votes := hs.verifiedVotes[cert.BlockHash()]
	votes = append(votes, cert)
	hs.verifiedVotes[cert.BlockHash()] = votes

	if len(votes) < hs.mod.Config().QuorumSize() {
		return
	}

	qc, err := hs.mod.Signer().CreateQuorumCert(block, votes)
	if err != nil {
		logger.Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(hs.verifiedVotes, cert.BlockHash())
	hs.updateHighQC(qc)

	// signal the synchronizer
	hs.mod.ViewSynchronizer().AdvanceView(hotstuff.SyncInfo{QC: qc})
}

func (hs *chainedhotstuff) deliver(block *hotstuff.Block) {
	votes, ok := hs.pendingVotes[block.Hash()]
	if !ok {
		return
	}

	logger.Debugf("OnDeliver: %v", block)

	delete(hs.pendingVotes, block.Hash())

	hs.mod.BlockChain().Store(block)

	for _, vote := range votes {
		hs.mod.EventLoop().AddEvent(vote)
	}
}

// OnDeliver handles an incoming block
func (hs *chainedhotstuff) OnDeliver(block *hotstuff.Block) {
	hs.deliver(block)
}

var _ hotstuff.Consensus = (*chainedhotstuff)(nil)
