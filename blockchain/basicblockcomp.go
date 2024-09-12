package blockchain

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipelining"
)

type basicBlockComp struct {
	blockChain  modules.BlockChain
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

func NewBasicBlockComp() modules.BlockCompositor {
	return &basicBlockComp{
		bExec: hotstuff.GetGenesis(),
	}
}

func (bb *basicBlockComp) InitModule(mods *modules.Core, _ modules.InitOptions) {
	mods.Get(
		&bb.executor,
		&bb.blockChain,
		&bb.forkHandler,
		&bb.logger,
	)
}

// Stores the block before further execution.
func (bb *basicBlockComp) Store(block *hotstuff.Block) {
	bb.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := bb.commitInner(block)
	bb.mut.Unlock()

	if err != nil {
		bb.logger.Warnf("failed to commit: %v", err)
		return
	}

	// cs.commitComp.Commit(block)
	// prune the blockchain and handle forked blocks
	forkedBlocks := bb.blockChain.PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		bb.forkHandler.Fork(block)
	}
}

// recursive helper for commit
func (bb *basicBlockComp) commitInner(block *hotstuff.Block) error {
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
func (bb *basicBlockComp) CommittedBlock(_ pipelining.PipeId) *hotstuff.Block {
	return bb.bExec
}
