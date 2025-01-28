package certauth

import (
	"container/list"
	"crypto/sha256"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/netconfig"
)

type Cache struct {
	impl        modules.CryptoBase
	mut         sync.Mutex
	capacity    int
	entries     map[string]*list.Element
	accessOrder list.List
}

// NewCached returns a new CertAuth instance that caches the results of the operations of the given CryptoBase.
// implementation.
func NewCached(
	impl modules.CryptoBase,
	blockChain *blockchain.BlockChain,
	configuration *netconfig.Config,
	logger logging.Logger,

	capacity int) *CertAuthority {
	return New(&Cache{
		impl:     impl,
		capacity: capacity,
		entries:  make(map[string]*list.Element, capacity),
	},
		blockChain,
		configuration,
		logger,
	)
}

func (cache *Cache) insert(key string) {
	cache.mut.Lock()
	defer cache.mut.Unlock()
	elem, ok := cache.entries[key]
	if ok {
		cache.accessOrder.MoveToFront(elem)
		return
	}
	cache.evict()
	elem = cache.accessOrder.PushFront(key)
	cache.entries[key] = elem
}

func (cache *Cache) check(key string) bool {
	cache.mut.Lock()
	defer cache.mut.Unlock()
	elem, ok := cache.entries[key]
	if !ok {
		return false
	}
	cache.accessOrder.MoveToFront(elem)
	return true
}

func (cache *Cache) evict() {
	if len(cache.entries) < cache.capacity {
		return
	}
	key := cache.accessOrder.Remove(cache.accessOrder.Back()).(string)
	delete(cache.entries, key)
}

// Sign signs a message and adds it to the cache for use during verification.
func (cache *Cache) Sign(message []byte) (sig hotstuff.QuorumSignature, err error) {
	sig, err = cache.impl.Sign(message)
	if err != nil {
		return nil, err
	}
	var key strings.Builder
	hash := sha256.Sum256(message)
	_, _ = key.Write(hash[:])
	_, _ = key.Write(sig.ToBytes())
	cache.insert(key.String())
	return sig, nil
}

// Verify verifies the given quorum signature against the message.
func (cache *Cache) Verify(signature hotstuff.QuorumSignature, message []byte) bool {
	var key strings.Builder
	hash := sha256.Sum256(message)
	_, _ = key.Write(hash[:])
	_, _ = key.Write(signature.ToBytes())

	if cache.check(key.String()) {
		return true
	}

	if cache.impl.Verify(signature, message) {
		cache.insert(key.String())
		return true
	}

	return false
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (cache *Cache) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool {
	// sort the list of ids from the batch map
	ids := slices.Sorted(maps.Keys(batch))
	var hash hotstuff.Hash
	hasher := sha256.New()
	// then hash the messages in sorted order
	for _, id := range ids {
		_, _ = hasher.Write(batch[id])
	}
	hasher.Sum(hash[:])

	var key strings.Builder
	_, _ = key.Write(hash[:])
	_, _ = key.Write(signature.ToBytes())

	if cache.check(key.String()) {
		return true
	}

	if cache.impl.BatchVerify(signature, batch) {
		cache.insert(key.String())
		return true
	}

	return false
}

// Combine combines multiple signatures together into a single signature.
func (cache *Cache) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	// we don't cache the result of this operation, because it is not guaranteed to be valid.
	return cache.impl.Combine(signatures...)
}
