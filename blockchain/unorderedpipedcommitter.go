package blockchain

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
)

func init() {
	modules.RegisterModule("unordered", NewUnorderedPipedCommitter)
}

type unorderedPipedCommitter struct {
	blockChain  modules.BlockChain
	executor    modules.ExecutorExt
	forkHandler modules.ForkHandlerExt
	logger      logging.Logger

	mut         sync.Mutex
	bExecAtPipe map[pipeline.Pipe]*hotstuff.Block
	pipeCount   int
	currentView hotstuff.View
	currentPipe pipeline.Pipe
}

func NewUnorderedPipedCommitter() modules.BlockCommitter {
	return &unorderedPipedCommitter{
		bExecAtPipe: make(map[pipeline.Pipe]*hotstuff.Block),
		currentView: 1,
		currentPipe: 1,
	}
}

func (pc *unorderedPipedCommitter) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&pc.executor,
		&pc.blockChain,
		&pc.forkHandler,
		&pc.logger,
	)

	pc.pipeCount = opt.PipeCount
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			pc.bExecAtPipe[pipe] = hotstuff.GetGenesis()
		}
		return
	}

	pc.bExecAtPipe[pipeline.NullPipe] = hotstuff.GetGenesis()
}

// Stores the block before further execution.
func (pc *unorderedPipedCommitter) Commit(block *hotstuff.Block) {
	pc.logger.Debugf("Commit (currentPipe: %d, currentView: %d): new incoming block {p:%d, v:%d, h:%s}",
		pc.currentPipe, pc.currentView,
		block.Pipe(), block.View(), block.Hash().String()[:4])

	pc.bExecAtPipe[block.Pipe()] = block
	pc.executor.Exec(block)

	//pc.mut.Lock()
	//// can't recurse due to requiring the mutex, so we use a helper instead.
	//err := pc.commitInner(block)
	//pc.mut.Unlock()
	//
	//if err != nil {
	//	pc.logger.Debug("failed to commit block")
	//}
	//
	// prunedBlocks := pc.blockChain.PruneToHeight(block.View())
	// pc.handleForks(prunedBlocks)
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (pc *unorderedPipedCommitter) CommittedBlock(pipe pipeline.Pipe) *hotstuff.Block {
	pc.mut.Lock()
	defer pc.mut.Unlock()
	return pc.bExecAtPipe[pipe]
}

// recursive helper for commit
func (pc *unorderedPipedCommitter) commitInner(block *hotstuff.Block) error {
	if pc.bExecAtPipe[block.Pipe()].View() >= block.View() {
		return nil
	}

	if parent, ok := pc.blockChain.Get(block.Parent(), block.Pipe()); ok {
		err := pc.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}

	pc.logger.Debugf("commitInner: Executed block: {p:%d, v:%d, h:%s}", block.Pipe(), block.View(), block.Hash().String()[:4])
	pc.bExecAtPipe[block.Pipe()] = block
	pc.blockChain.DeleteAtHeight(block.View(), block.Hash())
	return nil
}

func (pc *unorderedPipedCommitter) handleForks(prunedBlocks map[hotstuff.View][]*hotstuff.Block) {
	// All pruned blocks are assumed to be forks after the previous exec logic
	for _, blocks := range prunedBlocks {
		for _, block := range blocks {
			pc.forkHandler.Fork(block)
		}
	}
}

var _ modules.BlockCommitter = (*unorderedPipedCommitter)(nil)
