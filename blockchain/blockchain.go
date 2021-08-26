// Package blockchain provides an implementation of the consensus.BlockChain interface.
package blockchain

import (
	"context"
	"sync"

	"github.com/relab/hotstuff/consensus"
)

// blockChain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type blockChain struct {
	mods          *consensus.Modules
	mut           sync.Mutex
	pruneHeight   consensus.View
	blocks        map[consensus.Hash]*consensus.Block
	blockAtHeight map[consensus.View]*consensus.Block
	pendingFetch  map[consensus.Hash]context.CancelFunc // allows a pending fetch operation to be cancelled
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (chain *blockChain) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	chain.mods = mods
}

// New creates a new blockChain with a maximum size.
// Blocks are dropped in least recently used order.
func New() consensus.BlockChain {
	bc := &blockChain{
		blocks:        make(map[consensus.Hash]*consensus.Block),
		blockAtHeight: make(map[consensus.View]*consensus.Block),
		pendingFetch:  make(map[consensus.Hash]context.CancelFunc),
	}
	bc.Store(consensus.GetGenesis())
	return bc
}

// Store stores a block in the blockchain
func (chain *blockChain) Store(block *consensus.Block) {
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
func (chain *blockChain) LocalGet(hash consensus.Hash) (*consensus.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	block, ok := chain.blocks[hash]
	if !ok {
		return nil, false
	}

	return block, true
}

// Get retrieves a block given its hash. Get will try to find the block locally.
// If it is not available locally, it will try to fetch the block.
func (chain *blockChain) Get(hash consensus.Hash) (block *consensus.Block, ok bool) {
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

	ctx, cancel = context.WithCancel(chain.mods.Synchronizer().ViewContext())
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.mods.Logger().Debugf("Attempting to fetch block: %.8s", hash)
	block, ok = chain.mods.Configuration().Fetch(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we we fetching
		block, ok = chain.blocks[hash]
		goto done
	}

	chain.mods.Logger().Debugf("Successfully fetched block: %.8s", hash)

	chain.blocks[hash] = block
	chain.blockAtHeight[block.View()] = block

done:
	defer chain.mut.Unlock()

	if !ok {
		return nil, false
	}

	return block, true
}

// Extends checks if the given block extends the branch of the target block.
func (chain *blockChain) Extends(block, target *consensus.Block) bool {
	current := block
	ok := true
	for ok && current.View() > target.View() {
		current, ok = chain.Get(current.Parent())
	}
	return ok && current.Hash() == target.Hash()
}

func (chain *blockChain) PruneToHeight(height consensus.View) (forkedBlocks []*consensus.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	committedHeight := chain.mods.Consensus().CommittedBlock().View()
	committedViews := make(map[consensus.View]bool)
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
				chain.mods.Logger().Debugf("PruneToHeight: found forked block: %v", block)
				forkedBlocks = append(forkedBlocks, block)
			}
		}
		delete(chain.blockAtHeight, h)
	}
	chain.pruneHeight = height
	return forkedBlocks
}

var _ consensus.BlockChain = (*blockChain)(nil)
