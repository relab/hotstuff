// Package blockchain provides an implementation of the consensus.BlockChain interface.
package blockchain

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff/consensus"
)

// blockChain stores a limited amount of blocks in a map.
// blocks are evicted in LRU order.
type blockChain struct {
	mod *consensus.Modules

	mut          sync.Mutex
	maxSize      int
	blocks       map[consensus.Hash]*list.Element
	pendingFetch map[consensus.Hash]context.CancelFunc // allows a pending fetch operation to be cancelled
	accessOrder  list.List
}

// InitModule gives the module a reference to the HotStuff object.
func (chain *blockChain) InitModule(hs *consensus.Modules, _ *consensus.OptionsBuilder) {
	chain.mod = hs

	chain.mod.EventLoop().RegisterAsyncObserver(consensus.ProposeMsg{}, func(event interface{}) {
		proposal := event.(consensus.ProposeMsg)
		chain.Store(proposal.Block)
	})
}

// New creates a new blockChain with a maximum size.
// Blocks are dropped in least recently used order.
func New(maxSize int) consensus.BlockChain {
	bc := &blockChain{
		maxSize:      maxSize,
		blocks:       make(map[consensus.Hash]*list.Element),
		pendingFetch: make(map[consensus.Hash]context.CancelFunc),
	}
	bc.Store(consensus.GetGenesis())
	return bc
}

func (chain *blockChain) makeSpace() {
	if len(chain.blocks) < chain.maxSize {
		return
	}
	elem := chain.accessOrder.Back()
	block := elem.Value.(*consensus.Block)
	delete(chain.blocks, block.Hash())
	chain.accessOrder.Remove(elem)
}

// Store stores a block in the blockchain
func (chain *blockChain) Store(block *consensus.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	chain.makeSpace()

	elem := chain.accessOrder.PushFront(block)
	chain.blocks[block.Hash()] = elem

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
	}
}

// Get retrieves a block given its hash. It will only try the local cache.
func (chain *blockChain) LocalGet(hash consensus.Hash) (*consensus.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	elem, ok := chain.blocks[hash]
	if !ok {
		return nil, false
	}

	chain.accessOrder.MoveToFront(elem)

	return elem.Value.(*consensus.Block), true
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
	elem, ok := chain.blocks[hash]
	if ok {
		goto done
	}

	ctx, cancel = context.WithCancel(chain.mod.Synchronizer().ViewContext())
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	chain.mod.Logger().Debugf("Attempting to fetch block: %.8s", hash)
	block, ok = chain.mod.Configuration().Fetch(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we we fetching
		elem, ok = chain.blocks[hash]
		goto done
	}

	chain.mod.Logger().Debugf("Successfully fetched block: %.8s", hash)

	chain.makeSpace()
	elem = chain.accessOrder.PushFront(block)
	chain.blocks[hash] = elem

done:
	defer chain.mut.Unlock()

	if !ok {
		return nil, false
	}

	chain.accessOrder.MoveToFront(elem)
	return elem.Value.(*consensus.Block), true
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

var _ consensus.BlockChain = (*blockChain)(nil)
