package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
)

type Committer struct {
	blockChain  *blockchain.BlockChain
	executor    core.ExecutorExt
	forkHandler core.ForkHandlerExt
	logger      logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// Basic committer implements commit logic without pipelining.
func New() *Committer {
	return &Committer{
		bExec: hotstuff.GetGenesis(),
	}
}

func (bb *Committer) InitModule(mods *core.Core) {
	mods.Get(
		&bb.executor,
		&bb.blockChain,
		&bb.forkHandler,
		&bb.logger,
	)
}

// Stores the block before further execution.
func (bb *Committer) Commit(committedHeight hotstuff.View, block *hotstuff.Block) {
	err := bb.commit(block)
	if err != nil {
		bb.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	prunedBlocks := bb.blockChain.PruneToHeight(block.View())
	forkedBlocks := bb.findForks(committedHeight, block.View(), prunedBlocks)
	for _, blocks := range forkedBlocks {
		bb.forkHandler.Fork(blocks)
	}
}

func (bb *Committer) commit(block *hotstuff.Block) error {
	bb.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := bb.commitInner(block)
	bb.mut.Unlock()
	return err
}

// recursive helper for commit
func (bb *Committer) commitInner(block *hotstuff.Block) error {
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
func (bb *Committer) CommittedBlock() *hotstuff.Block {
	bb.mut.Lock()
	defer bb.mut.Unlock()
	return bb.bExec
}

func (bb *Committer) findForks(committedHeight, height hotstuff.View, blocksAtHeight map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {

	committedViews := make(map[hotstuff.View]bool)

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
