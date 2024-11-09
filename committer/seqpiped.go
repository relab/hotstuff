package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/debug"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
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
	bExecAtPipe         map[pipeline.Pipe]*hotstuff.Block
	waitingBlocksAtPipe map[pipeline.Pipe][]*hotstuff.Block
	pipeCount           int
	currentView         hotstuff.View
	currentPipe         pipeline.Pipe
}

func NewSequentiallyPiped() modules.Committer {
	return &sequentiallyPiped{
		bExecAtPipe:         make(map[pipeline.Pipe]*hotstuff.Block),
		waitingBlocksAtPipe: make(map[pipeline.Pipe][]*hotstuff.Block),
		currentView:         1,
		currentPipe:         1,
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

	sp.pipeCount = opt.PipeCount
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			sp.bExecAtPipe[pipe] = hotstuff.GetGenesis()
			sp.waitingBlocksAtPipe[pipe] = nil
		}
		return
	}

	sp.bExecAtPipe[pipeline.NullPipe] = hotstuff.GetGenesis()
	sp.waitingBlocksAtPipe[pipeline.NullPipe] = nil
}

// Stores the block before further execution.
func (sp *sequentiallyPiped) Commit(block *hotstuff.Block) {
	sp.logger.Debugf("Commit (currentPipe: %d, currentView: %d): new incoming block {p:%d, v:%d, h:%s}",
		sp.currentPipe, sp.currentView,
		block.Pipe(), block.View(), block.Hash().String()[:4])
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

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (sp *sequentiallyPiped) CommittedBlock(pipe pipeline.Pipe) *hotstuff.Block {
	sp.mut.Lock()
	defer sp.mut.Unlock()
	return sp.bExecAtPipe[pipe]
}

// recursive helper for commit
func (sp *sequentiallyPiped) commitInner(block *hotstuff.Block) error {
	if sp.bExecAtPipe[block.Pipe()].View() >= block.View() {
		return nil
	}

	// Check if the block was added to the end of the queue. If so, exit.
	blockQueue := sp.waitingBlocksAtPipe[block.Pipe()]
	if len(blockQueue) > 0 && blockQueue[len(blockQueue)-1].View() >= block.View() {
		return nil
	}

	if parent, ok := sp.blockChain.Get(block.Parent(), block.Pipe()); ok {
		err := sp.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}

	sp.logger.Debugf("commitInner: Queued block: {p:%d, v:%d, h:%s}", block.Pipe(), block.View(), block.Hash().String()[:4])
	sp.waitingBlocksAtPipe[block.Pipe()] = append(sp.waitingBlocksAtPipe[block.Pipe()], block)
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
	waitingBlocks := sp.waitingBlocksAtPipe[sp.currentPipe]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		sp.logger.Debugf("tryExec (currentPipe: %d, currentView: %d): no block on pipe yet", sp.currentPipe, sp.currentView)
		return nil
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == sp.currentView {
		// Execute block
		sp.logger.Debugf("tryExec: block executed: {p=%d, v=%d, h:%s}", peekedBlock.Pipe(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		sp.executor.Exec(peekedBlock)
		sp.bExecAtPipe[peekedBlock.Pipe()] = peekedBlock
		// Pop from queue
		sp.waitingBlocksAtPipe[sp.currentPipe] = sp.waitingBlocksAtPipe[sp.currentPipe][1:]
		// Delete from chain.
		err := sp.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
		if err != nil {
			return err
		}
	} else {
		sp.logger.Debugf("tryExec (currentPipe: %d, currentView: %d): block in queue does not match view: {p:%d, v:%d, h:%s}",
			sp.currentPipe, sp.currentView,
			peekedBlock.Pipe(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		sp.eventLoop.DebugEvent(debug.CommitHaltEvent{OnPipe: sp.currentPipe})
	}

	sp.currentPipe++
	if sp.currentPipe == pipeline.Pipe(sp.pipeCount)+1 {
		sp.currentPipe = 1
		// Prune out remaining blocks in the chain. Those blocks are guaranteed to be forks.
		prunedBlocks := sp.blockChain.PruneToHeight(sp.currentView)
		sp.handleForks(prunedBlocks)
		sp.currentView++
		sp.logger.Debugf("tryExec (currentPipe: %d): advance to view %d", sp.currentPipe, sp.currentView)
	}

	return sp.tryExec()
}

var _ modules.Committer = (*sequentiallyPiped)(nil)
