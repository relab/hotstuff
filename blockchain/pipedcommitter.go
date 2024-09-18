package blockchain

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipelining"
)

type pipedCommitter struct {
	consensuses map[pipelining.PipeId]modules.Consensus
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

func (pc *pipedCommitter) InitModule(mods *modules.Core, opt modules.InitOptions) {
	mods.Get(
		&pc.executor,
		&pc.blockChain,
		&pc.forkHandler,
		&pc.logger,
	)

	pc.consensuses = make(map[pipelining.PipeId]modules.Consensus)
	if opt.IsPipeliningEnabled {
		for _, pipe := range mods.Pipes() {
			var cs modules.Consensus
			mods.MatchForPipe(pipe, cs)
			pc.consensuses[pipe] = cs
		}
		return
	}

	var cs modules.Consensus
	mods.Get(&cs)
	pc.consensuses[pipelining.NullPipeId] = cs
}

// Stores the block before further execution.
func (pc *pipedCommitter) Store(block *hotstuff.Block) {
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (pc *pipedCommitter) CommittedBlock(_ pipelining.PipeId) *hotstuff.Block {
	pc.mut.Lock()
	defer pc.mut.Unlock()
	return pc.bExec
}

func (pc *pipedCommitter) findForks(height hotstuff.View, blocksAtHeight map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {
	return nil
}

var _ modules.BlockCommitter = (*basicCommitter)(nil)
