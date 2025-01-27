// Package blockchain provides an implementation of the consensus.BlockChain interface.
package blockchain

import (
	"context"
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/sender"
	"github.com/relab/hotstuff/synctools"
)

// BlockChain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type BlockChain struct {
	sender    *sender.Sender
	eventLoop *eventloop.EventLoop
	logger    logging.Logger

	mut         sync.Mutex
	pruneHeight hotstuff.View
	blocks      map[hotstuff.Hash]*hotstuff.Block
	// blockAtHeight map[hotstuff.View][]*hotstuff.Block
	blockAtHeight map[hotstuff.View]*hotstuff.Block
	pendingFetch  map[hotstuff.Hash]context.CancelFunc // allows a pending fetch operation to be canceled
}

// New creates a new blockChain with a maximum size.
// Blocks are dropped in least recently used order.
func New(
	sender *sender.Sender,
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
) *BlockChain {
	bc := &BlockChain{
		sender:    sender,
		eventLoop: eventLoop,
		logger:    logger,

		blocks:        make(map[hotstuff.Hash]*hotstuff.Block),
		blockAtHeight: make(map[hotstuff.View]*hotstuff.Block),
		pendingFetch:  make(map[hotstuff.Hash]context.CancelFunc),
	}
	bc.Store(hotstuff.GetGenesis())
	return bc
}

// Store stores a block in the blockchain
func (chain *BlockChain) Store(block *hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	chain.blocks[block.Hash()] = block
	chain.blockAtHeight[block.View()] = block

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
	}
}

// Get retrieves a block given its hash. It will only try the local cache.
func (chain *BlockChain) LocalGet(hash hotstuff.Hash) (*hotstuff.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	block, ok := chain.blocks[hash]
	if !ok {
		return nil, false
	}

	return block, true
}

func (chain *BlockChain) DeleteAtHeight(height hotstuff.View, blockHash hotstuff.Hash) error {
	block, ok := chain.blockAtHeight[height]
	if !ok {
		return fmt.Errorf("no blocks at height %d", height)
	}

	strHash := blockHash.String()
	if block.Hash().String() == strHash {
		delete(chain.blockAtHeight, height)
		return nil
	}
	return fmt.Errorf("block not found at height %d", height)
}

// Get retrieves a block given its hash. Get will try to find the block locally.
// If it is not available locally, it will try to fetch the block.
func (chain *BlockChain) Get(hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
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

	ctx, cancel = synctools.TimeoutContext(chain.eventLoop.Context(), chain.eventLoop)
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.logger.Debugf("Attempting to fetch block: %.8s", hash)
	block, ok = chain.sender.Fetch(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we we fetching
		block, ok = chain.blocks[hash]
		goto done
	}

	chain.logger.Debugf("Successfully fetched block: %.8s", hash)

	chain.blocks[hash] = block
	chain.blockAtHeight[block.View()] = block

done:
	chain.mut.Unlock()

	if !ok {
		return nil, false
	}

	return block, true
}

// Extends checks if the given block extends the branch of the target block.
func (chain *BlockChain) Extends(block, target *hotstuff.Block) bool {
	current := block
	ok := true
	for ok && current.View() > target.View() {
		current, ok = chain.Get(current.Parent())
	}
	return ok && current.Hash() == target.Hash()
}

func (chain *BlockChain) PruneToHeight(committedHeight, height hotstuff.View) (forkedBlocks []*hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	committedViews := make(map[hotstuff.View]bool)
	committedViews[committedHeight] = true
	for h := committedHeight; h >= chain.pruneHeight; {
		block, ok := chain.blockAtHeight[h]
		if !ok {
			break
		}
		parent, ok := chain.blocks[block.Parent()]
		if !ok || parent.View() < chain.pruneHeight {
			break
		}
		h = parent.View()
		committedViews[h] = true
	}

	for h := height; h > chain.pruneHeight; h-- {
		if !committedViews[h] {
			block, ok := chain.blockAtHeight[h]
			if ok {
				chain.logger.Debugf("PruneToHeight: found forked block: %v", block)
				forkedBlocks = append(forkedBlocks, block)
			}
		}
		delete(chain.blockAtHeight, h)
	}
	chain.pruneHeight = height
	return forkedBlocks
}

func (chain *BlockChain) PruneHeight() hotstuff.View {
	return chain.pruneHeight
}
