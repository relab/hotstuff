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
		currentView:         1,
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

	pc.pipeCount = opt.PipeCount
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			pc.bExecAtPipe[pipe] = hotstuff.GetGenesis()
			pc.waitingBlocksAtPipe[pipe] = nil
		}
		return
	}

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
	err = pc.tryExec()
	pc.mut.Unlock()
	if err != nil {
		pc.logger.Error(err)
	}
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (pc *waitingPipedCommitter) CommittedBlock(pipe pipeline.Pipe) *hotstuff.Block {
	pc.mut.Lock()
	defer pc.mut.Unlock()
	return pc.bExecAtPipe[pipe]
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

func (pc *waitingPipedCommitter) tryExec() error {
	waitingBlocks := pc.waitingBlocksAtPipe[pc.currentPipe]
	canPeek := len(waitingBlocks) > 0
	if !canPeek {
		return nil
	}

	peekedBlock := waitingBlocks[0]
	if peekedBlock.View() == pc.currentView {
		// Execute block
		pc.executor.Exec(peekedBlock)
		pc.bExecAtPipe[peekedBlock.Pipe()] = peekedBlock
		// Pop from queue
		pc.waitingBlocksAtPipe[pc.currentPipe] = pc.waitingBlocksAtPipe[pc.currentPipe][1:]
		// Delete from chain.
		err := pc.blockChain.DeleteAtHeight(peekedBlock.View(), peekedBlock.Hash())
		if err != nil {
			return err
		}
	}

	pc.currentPipe++
	if pc.currentPipe == pipeline.Pipe(pc.pipeCount)+1 {
		pc.currentPipe = 1
		pc.currentView++
	}

	return pc.tryExec()
}

var _ modules.BlockCommitter = (*basicCommitter)(nil)
