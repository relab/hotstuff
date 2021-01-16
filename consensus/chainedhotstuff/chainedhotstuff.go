package chainedhotstuff

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/internal/logging"
)

var logger = logging.GetLogger()

type chainedhotstuff struct {
	mut sync.Mutex

	// modular components
	cfg          hotstuff.Config
	commands     hotstuff.CommandQueue
	blocks       hotstuff.BlockChain
	signer       hotstuff.Signer
	verifier     hotstuff.Verifier
	executor     hotstuff.Executor
	synchronizer hotstuff.ViewSynchronizer

	// protocol variables

	view   hotstuff.View       // the last view that the replica voted in
	bLock  hotstuff.Block      // the currently locked block
	bExec  hotstuff.Block      // the last committed block
	bLeaf  hotstuff.Block      // the last proposed block
	highQC hotstuff.QuorumCert // the highest qc known to this replica

	pendingQCs map[hotstuff.Hash][]hotstuff.PartialCert
}

func (hs *chainedhotstuff) init() {
	hs.bLock = blockchain.GetGenesis()
	hs.bExec = blockchain.GetGenesis()
	hs.bLeaf = blockchain.GetGenesis()
	hs.highQC = blockchain.GetGenesis().QuorumCert()
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
	hs.mut.Lock()
	defer hs.mut.Unlock()

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

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		logger.Debug("DECIDE: ", block3)
		hs.commit(block3)
		hs.bExec = block3
	}
}

// Propose proposes the given command
func (hs *chainedhotstuff) Propose() {
	hs.mut.Lock()
	cmd := hs.commands.GetCommand()
	// TODO: Should probably use channels/contexts here instead such that
	// a proposal can be made a little later if a new command is added to the queue.
	// Alternatively, we could let the pacemaker know when commands arrive, so that it
	// can rall Propose() again.
	if cmd == nil {
		hs.mut.Unlock()
		return
	}
	block := blockchain.NewBlock(hs.bLeaf.Hash(), hs.highQC, *cmd, hs.bLeaf.View()+1, hs.cfg.ID())
	hs.mut.Unlock()

	hs.cfg.Propose(block)
	// self vote
	hs.OnPropose(block)
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

	hs.blocks.Store(block)
	hs.view = block.View()

	pc, err := hs.signer.Sign(block)
	if err != nil {
		logger.Error("OnPropose: failed to sign vote: ", err)
		return
	}

	leaderID := hs.leaderRotation.GetLeader(hs.view)
	if leaderID == hs.cfg.ID() {
		hs.mut.Unlock()
		hs.update(block)
		hs.OnVote(pc)
		return
	}

	// will do the update after the vote is sent
	defer hs.update(block)

	leader, ok := hs.cfg.Replicas()[leaderID]
	if !ok {
		logger.Panicf("Replica with ID %d was not found!", leaderID)
	}

	hs.mut.Unlock()
	leader.Vote(pc)
}

// OnVote handles an incoming vote
func (hs *chainedhotstuff) OnVote(cert hotstuff.PartialCert) {
	defer func() {
		hs.mut.Lock()
		// delete any pending QCs with lower height than bLeaf
		for k := range hs.pendingQCs {
			if block, ok := hs.blocks.Get(k); ok {
				if block.View() <= hs.bLeaf.View() {
					delete(hs.pendingQCs, k)
				}
			} else {
				delete(hs.pendingQCs, k)
			}
		}
		hs.mut.Unlock()
	}()

	if !hs.verifier.VerifyPartialCert(cert) {
		logger.Info("OnVote: Vote could not be verified!")
		return
	}

	logger.Debugf("OnVote: %.8s", cert.BlockHash())

	hs.mut.Lock()
	defer hs.mut.Unlock()

	block, ok := hs.blocks.Get(cert.BlockHash())
	if !ok {
		logger.Info("OnVote: could not find block for certificate.")
		return
	}

	votes, ok := hs.pendingQCs[cert.BlockHash()]
	if !ok {
		if block.View() <= hs.bLeaf.View() {
			return
		}
		hs.pendingQCs[cert.BlockHash()] = []hotstuff.PartialCert{cert}
	}

	if len(votes) >= hs.cfg.QuorumSize() {
		qc, err := hs.signer.CreateQuorumCert(block, votes)
		if err != nil {
			logger.Info("OnVote: could not create QC for block: ", err)
		}
		delete(hs.pendingQCs, cert.BlockHash())
		hs.updateHighQC(qc)
	}
}

// OnNewView handles an incoming NewView
func (hs *chainedhotstuff) OnNewView(qc hotstuff.QuorumCert) {
	hs.mut.Lock()
	defer hs.mut.Unlock()

	logger.Debug("OnNewView: ", qc)
	hs.updateHighQC(qc)
}

var _ hotstuff.Consensus = (*chainedhotstuff)(nil)
