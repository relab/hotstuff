package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/clientsrv"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
)

type Committer struct {
	blockChain *blockchain.BlockChain
	clientSrv  *clientsrv.ClientServer
	logger     logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// Basic committer implements commit logic without pipelining.
func New(
	blockChain *blockchain.BlockChain,
	clientSrv *clientsrv.ClientServer,
	logger logging.Logger,
) *Committer {
	return &Committer{
		blockChain: blockChain,
		clientSrv:  clientSrv,
		logger:     logger,

		bExec: hotstuff.GetGenesis(),
	}
}

// Stores the block before further execution.
func (cm *Committer) Commit(committedHeight hotstuff.View, block *hotstuff.Block) {
	err := cm.commit(block)
	if err != nil {
		cm.logger.Warnf("failed to commit: %v", err)
		return
	}

	// prune the blockchain and handle forked blocks
	prunedBlocks := cm.blockChain.PruneToHeight(block.View())
	forkedBlocks := cm.findForks(committedHeight, block.View(), prunedBlocks)
	for _, block := range forkedBlocks {
		cm.clientSrv.Fork(block.Command())
	}
}

func (cm *Committer) commit(block *hotstuff.Block) error {
	cm.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := cm.commitInner(block)
	cm.mut.Unlock()
	return err
}

// recursive helper for commit
func (cm *Committer) commitInner(block *hotstuff.Block) error {
	if cm.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := cm.blockChain.Get(block.Parent()); ok {
		err := cm.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cm.logger.Debug("EXEC: ", block)
	cm.clientSrv.Exec(block.Command())
	cm.bExec = block
	return nil
}

// Retrieve the last block which was committed on a pipe. Use zero if pipelining is not used.
func (cm *Committer) CommittedBlock() *hotstuff.Block {
	cm.mut.Lock()
	defer cm.mut.Unlock()
	return cm.bExec
}

func (cm *Committer) findForks(committedHeight, height hotstuff.View, blocksAtHeight map[hotstuff.View][]*hotstuff.Block) (forkedBlocks []*hotstuff.Block) {

	committedViews := make(map[hotstuff.View]bool)

	// This is a hacky value: chain.prevPruneHeight, but it works.
	for h := committedHeight; h >= cm.blockChain.PruneHeight(); {
		blocks, ok := blocksAtHeight[h]
		if !ok {
			break
		}
		block := blocks[0]
		parent, ok := cm.blockChain.LocalGet(block.Parent())
		if !ok || parent.View() < cm.blockChain.PruneHeight() {
			break
		}
		h = parent.View()
		committedViews[h] = true
	}

	for h := height; h > cm.blockChain.PruneHeight(); h-- {
		if !committedViews[h] {
			blocks, ok := blocksAtHeight[h]
			if ok {
				cm.logger.Debugf("PruneToHeight: found forked blocks: %v", blocks)
				block := blocks[0]
				forkedBlocks = append(forkedBlocks, block)
			}
		}
	}
	return
}

var _ core.Committer = (*Committer)(nil)
