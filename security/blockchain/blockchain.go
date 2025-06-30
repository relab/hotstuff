// Package blockchain provides a blockchain implementation.
package blockchain

import (
	"context"
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// Blockchain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type Blockchain struct {
	sender    core.Sender
	eventLoop *eventloop.EventLoop
	logger    logging.Logger

	mut         sync.Mutex
	pruneHeight hotstuff.View
	blocks      map[hotstuff.Hash]*hotstuff.Block
	// blockAtHeight map[hotstuff.View][]*hotstuff.Block
	blockAtHeight map[hotstuff.View]*hotstuff.Block
	pendingFetch  map[hotstuff.Hash]context.CancelFunc // allows a pending fetch operation to be canceled
}

// New creates a new blockchain with a maximum size.
// Blocks are dropped in least recently used order.
func New(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	sender core.Sender,
) *Blockchain {
	bc := &Blockchain{
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
func (chain *Blockchain) Store(block *hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	// do not store existing blocks, otherwise something is terribly wrong.
	if _, ok := chain.blocks[block.Hash()]; ok {
		chain.logger.Warnf("block already exists: %s", block.String())
		return
	}

	chain.blocks[block.Hash()] = block
	chain.blockAtHeight[block.View()] = block

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
	}
}

// LocalGet returns the block for the given hash from the local cache.
func (chain *Blockchain) LocalGet(hash hotstuff.Hash) (*hotstuff.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	block, ok := chain.blocks[hash]
	if !ok {
		return nil, false
	}
	return block, true
}

// DeleteAtHeight deletes the block with the provided block hash at the given height.
func (chain *Blockchain) DeleteAtHeight(height hotstuff.View, blockHash hotstuff.Hash) error {
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
func (chain *Blockchain) Get(hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
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

	ctx, cancel = chain.eventLoop.TimeoutContext()
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.logger.Debugf("Attempting to fetch block: %s", hash.SmallString())
	block, ok = chain.sender.RequestBlock(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we were fetching
		block, ok = chain.blocks[hash]
		goto done
	}

	chain.logger.Debugf("Successfully fetched block: %s", hash.SmallString())

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
func (chain *Blockchain) Extends(block, target *hotstuff.Block) bool {
	current := block
	ok := true
	for ok && current.View() > target.View() {
		current, ok = chain.Get(current.Parent())
	}
	return ok && current.Hash() == target.Hash()
}

// PruneToHeight prunes the blockchain to the given height.
func (chain *Blockchain) PruneToHeight(committedHeight, height hotstuff.View) (forkedBlocks []*hotstuff.Block) {
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

// PruneHeight returns the current prune height of the blockchain.
func (chain *Blockchain) PruneHeight() hotstuff.View {
	return chain.pruneHeight
}
