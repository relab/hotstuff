package committer

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/clientsrv"
)

type Committer struct {
	blockChain *blockchain.BlockChain
	clientSrv  *clientsrv.Server
	logger     logging.Logger

	mut   sync.Mutex
	bExec *hotstuff.Block
}

// Basic committer implements commit logic without pipelining.
func New(
	blockChain *blockchain.BlockChain,
	clientSrv *clientsrv.Server,
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
func (cm *Committer) Commit(block *hotstuff.Block) {
	err := cm.commit(block)
	if err != nil {
		cm.logger.Warnf("failed to commit: %v", err)
		return
	}

	forkedBlocks := cm.blockChain.PruneToHeight(
		cm.CommittedBlock().View(),
		block.View(),
	)
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
