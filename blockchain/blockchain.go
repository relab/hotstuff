// Package blockchain provides an implementation of the consensus.BlockChain interface.
package blockchain

import (
	"context"
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// blockChain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type blockChain struct {
	configuration modules.Configuration
	eventLoop     *eventloop.ScopedEventLoop
	logger        logging.Logger

	mut         sync.Mutex
	pruneHeight hotstuff.View
	blocks      map[hotstuff.Hash]*hotstuff.Block
	// blocksAtHeight map[hotstuff.View][]*hotstuff.Block
	blocksAtHeight map[hotstuff.View][]*hotstuff.Block
	pendingFetch   map[hotstuff.Hash]context.CancelFunc // allows a pending fetch operation to be canceled
}

func (chain *blockChain) InitModule(mods *modules.Core, _ modules.ScopeInfo) {
	mods.Get(
		&chain.configuration,
		&chain.eventLoop,
		&chain.logger,
	)
}

// New creates a new blockChain with a maximum size.
// Blocks are dropped in least recently used order.
func New() modules.BlockChain {
	bc := &blockChain{
		blocks:         make(map[hotstuff.Hash]*hotstuff.Block),
		blocksAtHeight: make(map[hotstuff.View][]*hotstuff.Block),
		pendingFetch:   make(map[hotstuff.Hash]context.CancelFunc),
	}
	bc.Store(hotstuff.GetGenesis())
	return bc
}

// Store stores a block in the blockchain
func (chain *blockChain) Store(block *hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	chain.blocks[block.Hash()] = block
	chain.blocksAtHeight[block.View()] = append(chain.blocksAtHeight[block.View()], block)

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
	}
}

// Get retrieves a block given its hash. It will only try the local cache.
func (chain *blockChain) LocalGet(hash hotstuff.Hash) (*hotstuff.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	block, ok := chain.blocks[hash]
	if !ok {
		return nil, false
	}

	return block, true
}

func (chain *blockChain) DeleteAtHeight(height hotstuff.View, blockHash hotstuff.Hash) error {
	blocks, ok := chain.blocksAtHeight[height]
	if !ok {
		return fmt.Errorf("no blocks at height %d", height)
	}

	strHash := blockHash.String()
	for i, block := range blocks {
		if block.Hash().String() == strHash {
			chain.blocksAtHeight[height] = append(chain.blocksAtHeight[height][:i], chain.blocksAtHeight[height][i+1:]...)
			if len(blocks) == 0 {
				delete(chain.blocksAtHeight, height)
			}
			return nil
		}
	}
	return fmt.Errorf("block not found at height %d", height)
}

// Get retrieves a block given its hash. Get will try to find the block locally.
// If it is not available locally, it will try to fetch the block.
func (chain *blockChain) Get(hash hotstuff.Hash, pipe hotstuff.Pipe) (block *hotstuff.Block, ok bool) {
	// need to declare vars early, or else we won't be able to use goto
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	chain.mut.Lock()
	block, ok = chain.blocks[hash]
	if ok {
		goto done
	}

	ctx, cancel = synchronizer.ScopedTimeoutContext(chain.eventLoop.Context(), chain.eventLoop, pipe)
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.logger.Debugf("Attempting to fetch block: %.8s", hash)
	block, ok = chain.configuration.Fetch(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we we fetching
		block, ok = chain.blocks[hash]
		goto done
	}

	chain.logger.Debugf("Successfully fetched block: %.8s", hash)

	chain.blocks[hash] = block
	chain.blocksAtHeight[block.View()] = append(chain.blocksAtHeight[block.View()], block)

done:
	chain.mut.Unlock()

	if !ok {
		return nil, false
	}

	return block, true
}

// Extends checks if the given block extends the branch of the target block.
func (chain *blockChain) Extends(block, target *hotstuff.Block) bool {
	current := block
	ok := true
	for ok && current.View() > target.View() {
		current, ok = chain.Get(current.Parent(), block.Pipe())
	}
	return ok && current.Hash() == target.Hash()
}

func (chain *blockChain) PruneToHeight(height hotstuff.View) (prunedBlocks map[hotstuff.View][]*hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()
	prunedBlocks = make(map[hotstuff.View][]*hotstuff.Block)

	for h := height; h > chain.pruneHeight; h-- {
		blocks, ok := chain.blocksAtHeight[h]
		if !ok {
			continue
		}
		for _, block := range blocks {
			// Add pruned blocks to list and go back a height
			// prunedBlocks = append(prunedBlocks, block)
			prunedBlocks[block.View()] = append(prunedBlocks[block.View()], block)
		}
		delete(chain.blocksAtHeight, h)
	}

	chain.pruneHeight = height

	return
}

func (chain *blockChain) PruneHeight() hotstuff.View {
	return chain.pruneHeight
}

var _ modules.BlockChain = (*blockChain)(nil)
