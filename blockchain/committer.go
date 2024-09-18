package blockchain

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipelining"
)

type basicCommitter struct {
	consensuses map[pipelining.PipeId]modules.Consensus
	blockChain  modules.BlockChain
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

func NewBasicCommitter() modules.BlockCommitter {
	return &basicCommitter{
		bExec: hotstuff.GetGenesis(),
	}
}

func (bb *basicCommitter) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&bb.executor,
		&bb.blockChain,
		&bb.forkHandler,
		&bb.logger,
	)

	bb.consensuses = make(map[pipelining.PipeId]modules.Consensus)
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			var cs modules.Consensus
			mods.MatchForPipe(pipe, cs)
			bb.consensuses[pipe] = cs
		}
		return
	}

	var cs modules.Consensus
	mods.Get(&cs)
	bb.consensuses[pipelining.NullPipeId] = cs
}

// Stores the block before further execution.
func (bb *basicCommitter) Store(block *hotstuff.Block) {
	err := bb.commit(block)
	if err != nil {
		bb.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	prunedBlocks := bb.blockChain.PruneToHeight(block.View())
	forkedBlocks := bb.FindForks(block.View(), prunedBlocks)
	for _, blocks := range forkedBlocks {
		bb.forkHandler.Fork(blocks)
	}
}

func (bb *basicCommitter) commit(block *hotstuff.Block) error {
	bb.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := bb.commitInner(block)
	bb.mut.Unlock()
	return err
}

// recursive helper for commit
func (bb *basicCommitter) commitInner(block *hotstuff.Block) error {
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
func (bb *basicCommitter) CommittedBlock(_ pipelining.PipeId) *hotstuff.Block {
	bb.mut.Lock()
	defer bb.mut.Unlock()
	return bb.bExec
}

func (bb *basicCommitter) FindForks(height hotstuff.View, blocksAtHeight map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {

	committedViews := make(map[hotstuff.View]bool)
	committedHeight := bb.consensuses[pipelining.NullPipeId].CommittedBlock().View()

	// TODO: This is a hacky value: chain.prevPruneHeight.
	for h := committedHeight; h >= bb.blockChain.PruneHeight(); {
		blocks, ok := blocksAtHeight[h]
		if !ok {
			break
		}
		// TODO: Support pipelined blocks. Right now it just takes the first one from the list in the view
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
			// TODO: Support pipelined blocks. Right now it just takes the first one from the list in the view
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

var _ modules.BlockCommitter = (*basicCommitter)(nil)
