package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

type basic struct {
	consensus   modules.Consensus
	blockChain  modules.BlockChain
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// Basic committer implements commit logic without pipelining.
func New() modules.Committer {
	return &basic{
		bExec: hotstuff.GetGenesis(),
	}
}

func (bb *basic) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	if info.IsPipeliningEnabled {
		panic("pipelining not supported for this module")
	}
	mods.Get(
		&bb.executor,
		&bb.blockChain,
		&bb.forkHandler,
		&bb.logger,
		&bb.consensus,
	)
}

// Stores the block before further execution.
func (bb *basic) Commit(block *hotstuff.Block) {
	err := bb.commit(block)
	if err != nil {
		bb.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	prunedBlocks := bb.blockChain.PruneToHeight(block.View())
	forkedBlocks := bb.findForks(block.View(), prunedBlocks)
	for _, blocks := range forkedBlocks {
		bb.forkHandler.Fork(blocks)
	}
}

func (bb *basic) commit(block *hotstuff.Block) error {
	bb.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := bb.commitInner(block)
	bb.mut.Unlock()
	return err
}

// recursive helper for commit
func (bb *basic) commitInner(block *hotstuff.Block) error {
	if bb.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := bb.blockChain.Get(block.Parent(), block.Pipe()); ok {
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
func (bb *basic) CommittedBlock(_ hotstuff.Pipe) *hotstuff.Block {
	bb.mut.Lock()
	defer bb.mut.Unlock()
	return bb.bExec
}

func (bb *basic) findForks(height hotstuff.View, blocksAtHeight map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {

	committedViews := make(map[hotstuff.View]bool)
	committedHeight := bb.consensus.CommittedBlock().View()

	// This is a hacky value: chain.prevPruneHeight, but it works.
	for h := committedHeight; h >= bb.blockChain.PruneHeight(); {
		blocks, ok := blocksAtHeight[h]
		if !ok {
			break
		}
		block := blocks[0]
		parent, ok := bb.blockChain.LocalGet(block.Parent())
		if !ok || parent.View() < bb.blockChain.PruneHeight() {
			break
		}
		h = parent.View()
		committedViews[h] = true
	}

	for h := height; h > bb.blockChain.PruneHeight(); h-- {
		if !committedViews[h] {
			blocks, ok := blocksAtHeight[h]
			if ok {
				bb.logger.Debugf("PruneToHeight: found forked blocks: %v", blocks)
				block := blocks[0]
				forkedBlocks = append(forkedBlocks, block)
			}
		}
	}
	return
}

var _ modules.Committer = (*basic)(nil)
