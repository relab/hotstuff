package chainedhotstuff

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/logging"
)

var logger = logging.GetLogger()

type chainedhotstuff struct {
	mut sync.Mutex

	// modular components
	cfg            hotstuff.Config
	blocks         hotstuff.BlockChain
	signer         hotstuff.Signer
	verifier       hotstuff.Verifier
	executor       hotstuff.Executor
	leaderRotation hotstuff.LeaderRotation

	// protocol variables

	view    hotstuff.View       // the last view that the replica voted in
	genesis hotstuff.Block      // the genesis block
	bLock   hotstuff.Block      // the currently locked block
	bExec   hotstuff.Block      // the last committed block
	bLeaf   hotstuff.Block      // the last proposed block
	highQC  hotstuff.QuorumCert // the highest qc known to this replica
}

func (hs *chainedhotstuff) init() {

}

// Config returns the configuration of this replica
func (hs *chainedhotstuff) Config() hotstuff.Config {
	return hs.cfg
}

// View returns the current view
func (hs *chainedhotstuff) View() hotstuff.View {
	hs.mut.Lock()
	defer hs.mut.Unlock()

	return hs.view
}

// HighQC returns the highest QC known to the replica
func (hs *chainedhotstuff) HighQC() hotstuff.QuorumCert {
	hs.mut.Lock()
	defer hs.mut.Unlock()

	return hs.highQC
}

// Leaf returns the last proposed block
func (hs *chainedhotstuff) Leaf() hotstuff.Block {
	hs.mut.Lock()
	defer hs.mut.Unlock()

	return hs.bLeaf
}

func (hs *chainedhotstuff) updateHighQC(qc hotstuff.QuorumCert) {
	if !hs.verifier.VerifyQuorumCert(qc) {
		logger.Info("updateQCHigh: QC could not be verified!")
	}

	newBlock, ok := hs.blocks.Get(qc.BlockHash())
	if !ok {
		logger.Info("updateQCHigh: Could not find block referenced by new QC!")
		return
	}

	oldBlock, ok := hs.blocks.Get(hs.highQC.BlockHash())
	if !ok {
		logger.Panic("Block from the old highQC missing from chain")
	}

	if newBlock.View() > oldBlock.View() {
		hs.highQC = qc
		hs.bLeaf = newBlock
	}
}

func (hs *chainedhotstuff) commit(block hotstuff.Block) {
	if hs.bExec.View() < block.View() {
		if parent, ok := hs.blocks.Get(block.Parent()); ok {
			hs.commit(parent)
		}
		logger.Debug("EXEC: ", block)
		hs.executor.Exec(block.Command())
	}
}

func (hs *chainedhotstuff) update(block hotstuff.Block) {
	block1, ok := hs.blocks.Get(block.QuorumCert().BlockHash())
	if !ok {
		return
	}

	logger.Debug("PRE_COMMIT: ", block1)
	hs.updateHighQC(block.QuorumCert())

	block2, ok := hs.blocks.Get(block1.QuorumCert().BlockHash())
	if !ok {
		return
	}

	if block2.View() > hs.bLock.View() {
		logger.Debug("COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.blocks.Get(block2.QuorumCert().BlockHash())
	if !ok {
		return
	}

	if *block1.Parent() == *block2.Hash() && *block2.Parent() == *block3.Hash() {
		logger.Debug("DECIDE: ", block3)
		hs.commit(block3)
		hs.bExec = block3
	}
}

// Propose proposes the given command
func (hs *chainedhotstuff) Propose(cmd hotstuff.Command) {
	panic("not implemented") // TODO: Implement
}

// OnPropose handles an incoming proposal
func (hs *chainedhotstuff) OnPropose(block hotstuff.Block) {
	logger.Debug("OnPropose: ", block)
	hs.mut.Lock()

	if block.View() <= hs.view {
		hs.mut.Unlock()
		logger.Info("OnPropose: block view was less than our view")
		return
	}

	qcBlock, haveQCBlock := hs.blocks.Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		logger.Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		b := block
		ok := true
		if ok && b.View() > hs.bLock.View()+1 {
			b, ok = hs.blocks.Get(b.Parent())
		}
		if ok && b.Parent() == hs.bLock.Hash() {
			safe = true
		} else {
			logger.Debug("OnPropose: safety condition failed")
		}
	}

	if !safe {
		hs.mut.Unlock()
		logger.Info("OnPropose: block not safe")
		return
	}

	hs.view = block.View()

}

// OnVote handles an incoming vote
func (hs *chainedhotstuff) OnVote(cert hotstuff.PartialCert) {
	panic("not implemented") // TODO: Implement
}

// OnVote handles an incoming NewView
func (hs *chainedhotstuff) OnNewView(qc hotstuff.QuorumCert) {
	panic("not implemented") // TODO: Implement
}

var _ hotstuff.Consensus = (*chainedhotstuff)(nil)
