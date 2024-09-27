package blockchain

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
)

type waitingPipedCommitter struct {
	consensuses map[pipeline.Pipe]modules.Consensus
	blockChain  modules.BlockChain
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

func NewWaitingPipedCommitter() modules.BlockCommitter {
	return &waitingPipedCommitter{
		bExecAtPipe:         make(map[pipeline.Pipe]*hotstuff.Block),
		waitingBlocksAtPipe: make(map[pipeline.Pipe][]*hotstuff.Block),
		currentPipe:         1,
	}
}

func (pc *waitingPipedCommitter) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&pc.executor,
		&pc.blockChain,
		&pc.forkHandler,
		&pc.logger,
	)

	pc.consensuses = make(map[pipeline.Pipe]modules.Consensus)
	pc.pipeCount = opt.PipeCount
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			var cs modules.Consensus
			mods.MatchForPipe(pipe, &cs)
			pc.consensuses[pipe] = cs
			pc.bExecAtPipe[pipe] = hotstuff.GetGenesis()
			pc.waitingBlocksAtPipe[pipe] = nil
		}
		return
	}

	var cs modules.Consensus
	mods.Get(&cs)
	pc.consensuses[pipeline.NullPipe] = cs
	pc.bExecAtPipe[pipeline.NullPipe] = hotstuff.GetGenesis()
	pc.waitingBlocksAtPipe[pipeline.NullPipe] = nil
}

// Stores the block before further execution.
func (pc *waitingPipedCommitter) Commit(block *hotstuff.Block) {
	pc.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := pc.commitInner(block)
	pc.mut.Unlock()

	if err != nil {
		pc.logger.Error("failed to commit block")
	}

	pc.mut.Lock()
	pc.tryExec()
	pc.mut.Unlock()
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (pc *waitingPipedCommitter) CommittedBlock(pipe pipeline.Pipe) *hotstuff.Block {
	pc.mut.Lock()
	defer pc.mut.Unlock()
	return pc.bExecAtPipe[pipe]
}

func (pc *waitingPipedCommitter) allPipesReady() bool {
	if pc.pipeCount == 0 {
		return true
	}

	count := 0
	for pipe := range pc.waitingBlocksAtPipe {
		if pc.waitingBlocksAtPipe[pipe] != nil {
			count++
		}
	}
	return pc.pipeCount == count
}

// recursive helper for commit
func (pc *waitingPipedCommitter) commitInner(block *hotstuff.Block) error {
	if pc.bExecAtPipe[block.Pipe()].View() >= block.View() {
		return nil
	}
	if parent, ok := pc.blockChain.Get(block.Parent()); ok {
		err := pc.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	pc.logger.Debug("VALID COMMIT: ", block)
	// pc.executor.Exec(block)
	// pc.bExecs[block.Pipe()] = block
	pc.waitingBlocksAtPipe[block.Pipe()] = append(pc.waitingBlocksAtPipe[block.Pipe()], block)
	return nil
}

func (pc *waitingPipedCommitter) tryExec() {
	waitingBlocks := pc.waitingBlocksAtPipe[pc.currentPipe]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		return
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == pc.currentView {
		// Execute block
		pc.executor.Exec(peekedBlock)
		pc.bExecAtPipe[peekedBlock.Pipe()] = peekedBlock
		// Pop from queue
		pc.waitingBlocksAtPipe[pc.currentPipe] = pc.waitingBlocksAtPipe[pc.currentPipe][1:]
		// Delete from chain. TODO (Alan): handle error
		_ = pc.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
	}

	pc.currentPipe++
	if pc.currentPipe == pipeline.Pipe(pc.pipeCount) {
		pc.currentPipe = 1
		pc.currentView++
	}

	pc.tryExec()
}

var _ modules.BlockCommitter = (*basicCommitter)(nil)
