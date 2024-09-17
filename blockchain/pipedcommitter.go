package blockchain

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipelining"
)

type pipedCommitter struct {
	blockChain  modules.BlockChain
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

func NewPipedCommitter() modules.BlockCommitter {
	return &pipedCommitter{
		bExec: hotstuff.GetGenesis(),
	}
}

func (bb *pipedCommitter) InitModule(mods *modules.Core, _ modules.InitOptions) {
	mods.Get(
		&bb.executor,
		&bb.blockChain,
		&bb.forkHandler,
		&bb.logger,
	)
}

// Stores the block before further execution.
func (bb *pipedCommitter) Store(block *hotstuff.Block) {
	bb.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := bb.commitInner(block)
	bb.mut.Unlock()

	if err != nil {
		bb.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	prunedBlocks := bb.blockChain.PruneToHeight(block.View())
	forkedBlocks := bb.blockChain.FindForks(prunedBlocks)
	for _, block := range forkedBlocks {
		bb.forkHandler.Fork(block)
	}
}

// recursive helper for commit
func (bb *pipedCommitter) commitInner(block *hotstuff.Block) error {
	if bb.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := bb.blockChain.Get(block.Parent()); ok {
		err := bb.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	bb.logger.Debug("EXEC: ", block)
	bb.executor.Exec(block)
	bb.bExec = block
	return nil
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (bb *pipedCommitter) CommittedBlock(_ pipelining.PipeId) *hotstuff.Block {
	bb.mut.Lock()
	defer bb.mut.Unlock()
	return bb.bExec
}
