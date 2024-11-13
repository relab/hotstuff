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
	modules.RegisterModule("sequential", NewMultiplexed)
}

type multiplexedCommitter struct {
	blockChain  modules.BlockChain
	eventLoop   *eventloop.ScopedEventLoop
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut                     sync.Mutex
	bExecAtCi               map[hotstuff.Instance]*hotstuff.Block
	waitingBlocksAtInstance map[hotstuff.Instance][]*hotstuff.Block
	instanceCount           int
	currentView             hotstuff.View
	currentInstance         hotstuff.Instance
}

func NewMultiplexed() modules.Committer {
	return &multiplexedCommitter{
		bExecAtCi:               make(map[hotstuff.Instance]*hotstuff.Block),
		waitingBlocksAtInstance: make(map[hotstuff.Instance][]*hotstuff.Block),
		currentView:             1,
		currentInstance:         1,
	}
}

func (c *multiplexedCommitter) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&c.eventLoop,
		&c.executor,
		&c.blockChain,
		&c.forkHandler,
		&c.logger,
	)

	c.instanceCount = opt.InstanceCount
	if opt.IsPipeliningEnabled {
		for _, instance := range mods.Scopes() {
			c.bExecAtCi[instance] = hotstuff.GetGenesis()
			c.waitingBlocksAtInstance[instance] = nil
		}
		return
	}

	c.bExecAtCi[hotstuff.ZeroInstance] = hotstuff.GetGenesis()
	c.waitingBlocksAtInstance[hotstuff.ZeroInstance] = nil
}

// Stores the block before further execution.
func (c *multiplexedCommitter) Commit(block *hotstuff.Block) {
	c.logger.Debugf("Commit (currentCi: %d, currentView: %d): new incoming block {p:%d, v:%d, h:%s}",
		c.currentInstance, c.currentView,
		block.Instance(), block.View(), block.Hash().String()[:4])
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

// Retrieve the last block which was committed on an instance. Use zero if pipelining is not used.
func (c *multiplexedCommitter) CommittedBlock(instance hotstuff.Instance) *hotstuff.Block {
	c.mut.Lock()
	defer c.mut.Unlock()
	return c.bExecAtCi[instance]
}

// recursive helper for commit
func (c *multiplexedCommitter) commitInner(block *hotstuff.Block) error {
	if c.bExecAtCi[block.Instance()].View() >= block.View() {
		return nil
	}

	// Check if the block was added to the end of the queue. If so, exit.
	blockQueue := c.waitingBlocksAtInstance[block.Instance()]
	if len(blockQueue) > 0 && blockQueue[len(blockQueue)-1].View() >= block.View() {
		return nil
	}

	if parent, ok := c.blockChain.Get(block.Parent(), block.Instance()); ok {
		err := c.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}

	c.logger.Debugf("commitInner: Queued block: {p:%d, v:%d, h:%s}", block.Instance(), block.View(), block.Hash().String()[:4])
	c.waitingBlocksAtInstance[block.Instance()] = append(c.waitingBlocksAtInstance[block.Instance()], block)
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
	waitingBlocks := c.waitingBlocksAtInstance[c.currentInstance]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		c.logger.Debugf("tryExec (currentCi: %d, currentView: %d): no block on instance yet", c.currentInstance, c.currentView)
		return nil
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == c.currentView {
		// Execute block
		c.logger.Debugf("tryExec: block executed: {p=%d, v=%d, h:%s}", peekedBlock.Instance(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		c.executor.Exec(peekedBlock)
		c.bExecAtCi[peekedBlock.Instance()] = peekedBlock
		// Pop from queue
		c.waitingBlocksAtInstance[c.currentInstance] = c.waitingBlocksAtInstance[c.currentInstance][1:]
		// Delete from chain.
		err := c.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
		if err != nil {
			return err
		}
	} else {
		c.logger.Debugf("tryExec (currentCi: %d, currentView: %d): block in queue does not match view: {p:%d, v:%d, h:%s}",
			c.currentInstance, c.currentView,
			peekedBlock.Instance(), peekedBlock.View(), peekedBlock.Hash().String()[:4])
		c.eventLoop.DebugEvent(debug.CommitHaltEvent{Instance: c.currentInstance})
	}

	c.currentInstance++
	if c.currentInstance == hotstuff.Instance(c.instanceCount)+1 {
		c.currentInstance = 1
		// Prune out remaining blocks in the chain. Those blocks are guaranteed to be forks.
		prunedBlocks := c.blockChain.PruneToHeight(c.currentView)
		c.handleForks(prunedBlocks)
		c.currentView++
		c.logger.Debugf("tryExec (currentCi: %d): advance to view %d", c.currentInstance, c.currentView)
	}

	return c.tryExec()
}

var _ modules.Committer = (*multiplexedCommitter)(nil)
