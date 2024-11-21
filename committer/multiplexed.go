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
	modules.RegisterModule("multiplexed", NewMultiplexed)
}

type multiplexedCommitter struct {
	blockChain  modules.BlockChain
	eventLoop   *eventloop.ScopedEventLoop
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut                 sync.Mutex
	bExecAtCi           map[hotstuff.Pipe]*hotstuff.Block
	waitingBlocksAtPipe map[hotstuff.Pipe][]*hotstuff.Block
	pipeCount           int
	currentView         hotstuff.View
	currentPipe         hotstuff.Pipe
}

// Multiplexed committer orders commits from multiple pipes by 1..n.
func NewMultiplexed() modules.Committer {
	return &multiplexedCommitter{
		bExecAtCi:           make(map[hotstuff.Pipe]*hotstuff.Block),
		waitingBlocksAtPipe: make(map[hotstuff.Pipe][]*hotstuff.Block),
		currentView:         1,
		currentPipe:         1,
	}
}

func (c *multiplexedCommitter) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	mods.Get(
		&c.eventLoop,
		&c.executor,
		&c.blockChain,
		&c.forkHandler,
		&c.logger,
	)

	c.pipeCount = info.ScopeCount
	if info.IsPipeliningEnabled {
		for _, pipe := range mods.Scopes() {
			c.bExecAtCi[pipe] = hotstuff.GetGenesis()
			c.waitingBlocksAtPipe[pipe] = nil
		}
		return
	}

	c.bExecAtCi[hotstuff.NullPipe] = hotstuff.GetGenesis()
	c.waitingBlocksAtPipe[hotstuff.NullPipe] = nil
}

// Stores the block before further execution.
func (c *multiplexedCommitter) Commit(block *hotstuff.Block) {
	c.logger.Debugf("Commit (currentCi: %d, currentView: %d): new incoming block {p:%d, v:%d, h:%s}",
		c.currentPipe, c.currentView,
		block.Pipe(), block.View(), block.Hash().String()[:4])
	c.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := c.commitInner(block)
	c.mut.Unlock()

	if err != nil {
		c.logger.Debug("failed to commit block")
	}

	c.mut.Lock()
	err = c.tryExec()
	c.mut.Unlock()
	if err != nil {
		c.logger.Debug(err)
	}
}

// Retrieve the last block which was committed on an pipe. Use zero if pipelining is not used.
func (c *multiplexedCommitter) CommittedBlock(pipe hotstuff.Pipe) *hotstuff.Block {
	c.mut.Lock()
	defer c.mut.Unlock()
	return c.bExecAtCi[pipe]
}

// recursive helper for commit
func (c *multiplexedCommitter) commitInner(block *hotstuff.Block) error {
	if c.bExecAtCi[block.Pipe()].View() >= block.View() {
		return nil
	}

	// Check if the block was added to the end of the queue. If so, exit.
	blockQueue := c.waitingBlocksAtPipe[block.Pipe()]
	if len(blockQueue) > 0 && blockQueue[len(blockQueue)-1].View() >= block.View() {
		return nil
	}

	if parent, ok := c.blockChain.Get(block.Parent(), block.Pipe()); ok {
		err := c.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}

	c.logger.Debugf("commitInner: Queued block: {p:%d, v:%d, h:%s}", block.Pipe(), block.View(), block.Hash().String()[:4])
	c.waitingBlocksAtPipe[block.Pipe()] = append(c.waitingBlocksAtPipe[block.Pipe()], block)
	return nil
}

func (c *multiplexedCommitter) handleForks(prunedBlocks map[hotstuff.View][]*hotstuff.Block) {
	// All pruned blocks are assumed to be forks after the previous exec logic
	for _, blocks := range prunedBlocks {
		for _, block := range blocks {
			c.forkHandler.Fork(block)
		}
	}
}

func (c *multiplexedCommitter) tryExec() error {
	waitingBlocks := c.waitingBlocksAtPipe[c.currentPipe]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		c.logger.Debugf("tryExec (currentCi: %d, currentView: %d): no block on pipe yet", c.currentPipe, c.currentView)
		return nil
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == c.currentView {
		// Execute block
		c.logger.Debugf("tryExec: block executed: {p=%d, v=%d, h:%s}", peekedBlock.Pipe(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		c.executor.Exec(peekedBlock)
		c.bExecAtCi[peekedBlock.Pipe()] = peekedBlock
		// Pop from queue
		c.waitingBlocksAtPipe[c.currentPipe] = c.waitingBlocksAtPipe[c.currentPipe][1:]
		// Delete from chain.
		err := c.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
		if err != nil {
			return err
		}
	} else {
		c.logger.Debugf("tryExec (currentCi: %d, currentView: %d): block in queue does not match view: {p:%d, v:%d, h:%s}",
			c.currentPipe, c.currentView,
			peekedBlock.Pipe(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		c.eventLoop.DebugEvent(debug.CommitHaltEvent{Pipe: c.currentPipe})
	}

	c.currentPipe++
	if c.currentPipe == hotstuff.Pipe(c.pipeCount)+1 {
		c.currentPipe = 1
		// Prune out remaining blocks in the chain. Those blocks are guaranteed to be forks.
		prunedBlocks := c.blockChain.PruneToHeight(c.currentView)
		c.handleForks(prunedBlocks)
		c.currentView++
		c.logger.Debugf("tryExec (currentCi: %d): advance to view %d", c.currentPipe, c.currentView)
	}

	return c.tryExec()
}

var _ modules.Committer = (*multiplexedCommitter)(nil)
