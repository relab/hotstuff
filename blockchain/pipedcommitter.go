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

	mut           sync.Mutex
	bExecs        map[pipeline.Pipe]*hotstuff.Block
	waitingBlocks map[pipeline.Pipe]*hotstuff.Block
	pipeCount     int
}

func NewWaitingPipedCommitter() modules.BlockCommitter {
	return &waitingPipedCommitter{
		bExecs:        make(map[pipeline.Pipe]*hotstuff.Block),
		waitingBlocks: make(map[pipeline.Pipe]*hotstuff.Block),
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
			pc.bExecs[pipe] = hotstuff.GetGenesis()
			pc.waitingBlocks[pipe] = nil
		}
		return
	}

	var cs modules.Consensus
	mods.Get(&cs)
	pc.consensuses[pipeline.NullPipe] = cs
	pc.bExecs[pipeline.NullPipe] = hotstuff.GetGenesis()
	pc.waitingBlocks[pipeline.NullPipe] = nil
}

// Stores the block before further execution.
func (pc *waitingPipedCommitter) Commit(block *hotstuff.Block) {
	pc.mut.Lock()
	defer pc.mut.Unlock()

	// The waiting process is simply adding new blocks
	// until we "fill all pipes" then start committing
	pc.waitingBlocks[block.Pipe()] = block
	if !pc.allPipesReady() {
		return
	}

	// Pipe IDs are already sorted, so we execute the blocks from
	// 1, 2, ..., N.
	for _, block := range pc.waitingBlocks {
		pc.commit(block)
	}

	// TODO: Handle forks for pipelines
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (pc *waitingPipedCommitter) CommittedBlock(pipe pipeline.Pipe) *hotstuff.Block {
	pc.mut.Lock()
	defer pc.mut.Unlock()
	return pc.bExecs[pipe]
}

func (pc *waitingPipedCommitter) allPipesReady() bool {
	if pc.pipeCount == 0 {
		return true
	}

	count := 0
	for pipe := range pc.waitingBlocks {
		if pc.waitingBlocks[pipe] != nil {
			count++
		}
	}
	return pc.pipeCount == count
}

// TODO: Implement
func (pc *waitingPipedCommitter) findForks(_ hotstuff.View, _ map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {
	return nil
}

func (pc *waitingPipedCommitter) commit(block *hotstuff.Block) error {
	pc.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := pc.commitInner(block)
	pc.mut.Unlock()
	return err
}

// recursive helper for commit
func (pc *waitingPipedCommitter) commitInner(block *hotstuff.Block) error {
	if pc.bExecs[block.Pipe()].View() >= block.View() {
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
	pc.logger.Debug("EXEC: ", block)
	pc.executor.Exec(block)
	pc.bExecs[block.Pipe()] = block
	return nil
}

var _ modules.BlockCommitter = (*basicCommitter)(nil)
