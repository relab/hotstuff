// Package blockchain provides an implementation of the consensus.BlockChain interface.
package blockchain

import (
	"context"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// blockChain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type blockChain struct {
	configuration modules.Configuration
	consensus     modules.Consensus
	synchronizer  modules.Synchronizer
	logger        logging.Logger
	mut           sync.Mutex
	pruneHeight   hotstuff.View
	blocks        map[hotstuff.ChainNumber]map[hotstuff.Hash]*hotstuff.Block
	blockAtHeight map[hotstuff.ChainNumber]map[hotstuff.View]*hotstuff.Block
	pendingFetch  map[hotstuff.Hash]context.CancelFunc // allows a pending fetch operation to be cancelled
}

func (chain *blockChain) InitModule(mods *modules.Core) {
	mods.Get(
		&chain.configuration,
		&chain.consensus,
		&chain.synchronizer,
		&chain.logger,
	)
}

// New creates a new blockChain with a maximum size.
// Blocks are dropped in least recently used order.
func New() modules.BlockChain {
	bc := &blockChain{
		blocks:        make(map[hotstuff.ChainNumber]map[hotstuff.Hash]*hotstuff.Block),
		blockAtHeight: make(map[hotstuff.ChainNumber]map[hotstuff.View]*hotstuff.Block),
		pendingFetch:  make(map[hotstuff.Hash]context.CancelFunc),
	}
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		bc.blocks[hotstuff.ChainNumber(i)] = make(map[hotstuff.Hash]*hotstuff.Block)
		bc.blockAtHeight[hotstuff.ChainNumber(i)] = make(map[hotstuff.View]*hotstuff.Block)
		bc.blocks[hotstuff.ChainNumber(i)][hotstuff.GetGenesis(hotstuff.ChainNumber(i)).Hash()] = hotstuff.GetGenesis(hotstuff.ChainNumber(i))
		bc.blockAtHeight[hotstuff.ChainNumber(i)][hotstuff.GetGenesis(hotstuff.ChainNumber(i)).View()] = hotstuff.GetGenesis(hotstuff.ChainNumber(i))
	}
	return bc
}

// Store stores a block in the blockchain
func (chain *blockChain) Store(block *hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	chain.blocks[block.ChainNumber()][block.Hash()] = block
	chain.blockAtHeight[block.ChainNumber()][block.View()] = block

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
	}
}

// Get retrieves a block given its hash. It will only try the local cache.
func (chain *blockChain) LocalGet(chainNumber hotstuff.ChainNumber, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	block, ok := chain.blocks[chainNumber][hash]
	if !ok {
		return nil, false
	}

	return block, true
}

// Get retrieves a block given its hash. Get will try to find the block locally.
// If it is not available locally, it will try to fetch the block.
func (chain *blockChain) Get(chainNumber hotstuff.ChainNumber, hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
	// need to declare vars early, or else we won't be able to use goto
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	chain.mut.Lock()
	block, ok = chain.blocks[chainNumber][hash]
	if ok {
		goto done
	}

	ctx, cancel = context.WithCancel(chain.synchronizer.ViewContext(chainNumber))
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.logger.Debugf("Attempting to fetch block: %.8s", hash)
	block, ok = chain.configuration.Fetch(ctx, chainNumber, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we we fetching
		block, ok = chain.blocks[chainNumber][hash]
		goto done
	}

	chain.logger.Debugf("Successfully fetched block: %.8s", hash)

	chain.blocks[chainNumber][hash] = block
	chain.blockAtHeight[chainNumber][block.View()] = block

done:
	defer chain.mut.Unlock()

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
		current, ok = chain.Get(block.ChainNumber(), current.Parent())
	}
	return ok && current.Hash() == target.Hash()
}

func (chain *blockChain) PruneToHeight(height hotstuff.View) (forkedBlocks []*hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		committedHeight := chain.consensus.CommittedBlock(hotstuff.ChainNumber(i)).View()
		committedViews := make(map[hotstuff.View]bool)
		committedViews[committedHeight] = true
		for h := committedHeight; h >= chain.pruneHeight; {
			block, ok := chain.blockAtHeight[hotstuff.ChainNumber(i)][h]
			if !ok {
				break
			}
			parent, ok := chain.blocks[hotstuff.ChainNumber(i)][block.Parent()]
			if !ok || parent.View() < chain.pruneHeight {
				break
			}
			h = parent.View()
			committedViews[h] = true
		}

		for h := height; h > chain.pruneHeight; h-- {
			if !committedViews[h] {
				block, ok := chain.blockAtHeight[hotstuff.ChainNumber(i)][h]
				if ok {
					chain.logger.Debugf("PruneToHeight: found forked block: %v", block)
					forkedBlocks = append(forkedBlocks, block)
				}
			}
			delete(chain.blockAtHeight[hotstuff.ChainNumber(i)], h)
		}
		chain.pruneHeight = height

	}
	return forkedBlocks
}

var _ modules.BlockChain = (*blockChain)(nil)
