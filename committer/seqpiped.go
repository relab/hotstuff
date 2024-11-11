package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/debug"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("sequential", NewSequentiallyPiped)
}

type sequentiallyPiped struct {
	blockChain  modules.BlockChain
	eventLoop   *eventloop.EventLoop
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut                 sync.Mutex
	bExecAtCi           map[hotstuff.Instance]*hotstuff.Block
	waitingBlocksAtPipe map[hotstuff.Instance][]*hotstuff.Block
	instanceCount       int
	currentView         hotstuff.View
	currentInstance     hotstuff.Instance
}

func NewSequentiallyPiped() modules.Committer {
	return &sequentiallyPiped{
		bExecAtCi:           make(map[hotstuff.Instance]*hotstuff.Block),
		waitingBlocksAtPipe: make(map[hotstuff.Instance][]*hotstuff.Block),
		currentView:         1,
		currentInstance:     1,
	}
}

func (sp *sequentiallyPiped) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&sp.eventLoop,
		&sp.executor,
		&sp.blockChain,
		&sp.forkHandler,
		&sp.logger,
	)

	sp.instanceCount = opt.InstanceCount
	if opt.IsPipeliningEnabled {
		for _, instance := range mods.Pipes() {
			sp.bExecAtCi[instance] = hotstuff.GetGenesis()
			sp.waitingBlocksAtPipe[instance] = nil
		}
		return
	}

	sp.bExecAtCi[hotstuff.ZeroInstance] = hotstuff.GetGenesis()
	sp.waitingBlocksAtPipe[hotstuff.ZeroInstance] = nil
}

// Stores the block before further execution.
func (sp *sequentiallyPiped) Commit(block *hotstuff.Block) {
	sp.logger.Debugf("Commit (currentCi: %d, currentView: %d): new incoming block {p:%d, v:%d, h:%s}",
		sp.currentInstance, sp.currentView,
		block.Instance(), block.View(), block.Hash().String()[:4])
	sp.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := sp.commitInner(block)
	sp.mut.Unlock()

	if err != nil {
		sp.logger.Debug("failed to commit block")
	}

	sp.mut.Lock()
	err = sp.tryExec()
	sp.mut.Unlock()
	if err != nil {
		sp.logger.Debug(err)
	}
}

// Retrieve the last block which was committed on an instance. Use zero if pipelining is not used.
func (sp *sequentiallyPiped) CommittedBlock(instance hotstuff.Instance) *hotstuff.Block {
	sp.mut.Lock()
	defer sp.mut.Unlock()
	return sp.bExecAtCi[instance]
}

// recursive helper for commit
func (sp *sequentiallyPiped) commitInner(block *hotstuff.Block) error {
	if sp.bExecAtCi[block.Instance()].View() >= block.View() {
		return nil
	}

	// Check if the block was added to the end of the queue. If so, exit.
	blockQueue := sp.waitingBlocksAtPipe[block.Instance()]
	if len(blockQueue) > 0 && blockQueue[len(blockQueue)-1].View() >= block.View() {
		return nil
	}

	if parent, ok := sp.blockChain.Get(block.Parent(), block.Instance()); ok {
		err := sp.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}

	sp.logger.Debugf("commitInner: Queued block: {p:%d, v:%d, h:%s}", block.Instance(), block.View(), block.Hash().String()[:4])
	sp.waitingBlocksAtPipe[block.Instance()] = append(sp.waitingBlocksAtPipe[block.Instance()], block)
	return nil
}

func (sp *sequentiallyPiped) handleForks(prunedBlocks map[hotstuff.View][]*hotstuff.Block) {
	// All pruned blocks are assumed to be forks after the previous exec logic
	for _, blocks := range prunedBlocks {
		for _, block := range blocks {
			sp.forkHandler.Fork(block)
		}
	}
}

func (sp *sequentiallyPiped) tryExec() error {
	waitingBlocks := sp.waitingBlocksAtPipe[sp.currentInstance]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		sp.logger.Debugf("tryExec (currentCi: %d, currentView: %d): no block on instance yet", sp.currentInstance, sp.currentView)
		return nil
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == sp.currentView {
		// Execute block
		sp.logger.Debugf("tryExec: block executed: {p=%d, v=%d, h:%s}", peekedBlock.Instance(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		sp.executor.Exec(peekedBlock)
		sp.bExecAtCi[peekedBlock.Instance()] = peekedBlock
		// Pop from queue
		sp.waitingBlocksAtPipe[sp.currentInstance] = sp.waitingBlocksAtPipe[sp.currentInstance][1:]
		// Delete from chain.
		err := sp.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
		if err != nil {
			return err
		}
	} else {
		sp.logger.Debugf("tryExec (currentCi: %d, currentView: %d): block in queue does not match view: {p:%d, v:%d, h:%s}",
			sp.currentInstance, sp.currentView,
			peekedBlock.Instance(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		sp.eventLoop.DebugEvent(debug.CommitHaltEvent{OnPipe: sp.currentInstance})
	}

	sp.currentInstance++
	if sp.currentInstance == hotstuff.Instance(sp.instanceCount)+1 {
		sp.currentInstance = 1
		// Prune out remaining blocks in the chain. Those blocks are guaranteed to be forks.
		prunedBlocks := sp.blockChain.PruneToHeight(sp.currentView)
		sp.handleForks(prunedBlocks)
		sp.currentView++
		sp.logger.Debugf("tryExec (currentCi: %d): advance to view %d", sp.currentInstance, sp.currentView)
	}

	return sp.tryExec()
}

var _ modules.Committer = (*sequentiallyPiped)(nil)
