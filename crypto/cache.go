package crypto

import (
	"container/list"
	"crypto/sha256"
	"github.com/relab/hotstuff/modules"
	"sort"
	"strings"
	"sync"

	"github.com/relab/hotstuff"
)

type cache struct {
	impl        modules.CryptoBase
	mut         sync.Mutex
	capacity    int
	entries     map[string]*list.Element
	accessOrder list.List
}

// NewCache returns a new Crypto implementation that caches the results of the operations of the given CryptoBase
// implementation.
func NewCache(impl modules.CryptoBase, capacity int) modules.Crypto {
	return New(&cache{
		impl:     impl,
		capacity: capacity,
		entries:  make(map[string]*list.Element, capacity),
	})
}

// InitConsensusModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (cache *cache) InitConsensusModule(mods *modules.ConsensusCore, cfg *modules.OptionsBuilder) {
	if mod, ok := cache.impl.(modules.Module); ok {
		mod.InitConsensusModule(mods, cfg)
	}
}

func (cache *cache) insert(key string) {
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

func (cache *cache) check(key string) bool {
	cache.mut.Lock()
	defer cache.mut.Unlock()
	elem, ok := cache.entries[key]
	if !ok {
		return false
	}
	cache.accessOrder.MoveToFront(elem)
	return true
}

func (cache *cache) evict() {
	if len(cache.entries) < cache.capacity {
		return
	}
	key := cache.accessOrder.Remove(cache.accessOrder.Back()).(string)
	delete(cache.entries, key)
}

// Sign signs a message and adds it to the cache for use during verification.
func (cache *cache) Sign(message []byte) (sig hotstuff.QuorumSignature, err error) {
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
func (cache *cache) Verify(signature hotstuff.QuorumSignature, message []byte) bool {
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
func (cache *cache) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) bool {
	// get a sorted list of the ids in the Messages map
	ids := make([]hotstuff.ID, 0, len(batch))
	for id := range batch {
		i := sort.Search(len(ids), func(i int) bool {
			return ids[i] < id
		})
		ids = append(ids, 0)
		copy(ids[i+1:], ids[i:])
		ids[i] = id
	}

	var hash hotstuff.Hash
	hasher := sha256.New()
	// then hash the messages in sorted order
	for _, id := range ids {
		m := batch[id]
		_, _ = hasher.Write(m)
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

// Verify verifies a signature given a hash.
// func (cache *cache) Verify(sig consensus.QuorumSignature, options ...consensus.VerifyOption) bool {
// 	if sig == nil {
// 		return false
// 	}

// 	var opts consensus.VerifyOptions
// 	for _, opt := range options {
// 		opt(&opts)
// 	}

// 	if sig.Participants().Len() < opts.Threshold {
// 		return false
// 	}

// 	// get a sorted list of the ids in the Messages map
// 	ids := make([]hotstuff.ID, 0, len(opts.Messages))
// 	for id := range opts.Messages {
// 		i := sort.Search(len(ids), func(i int) bool {
// 			return ids[i] < id
// 		})
// 		ids = append(ids, 0)
// 		copy(ids[i+1:], ids[i:])
// 		ids[i] = id
// 	}

// 	var hash consensus.Hash
// 	hasher := sha256.New()
// 	// then hash the messages in sorted order
// 	for _, id := range ids {
// 		m := opts.Messages[id]
// 		_, _ = hasher.Write(m)
// 	}
// 	hasher.Sum(hash[:])

// 	var key strings.Builder
// 	_, _ = key.Write(hash[:])
// 	_, _ = key.Write(sig.ToBytes())

// 	if cache.check(key.String()) {
// 		return true
// 	}

// 	if cache.impl.Verify(sig, options...) {
// 		cache.insert(key.String())
// 		return true
// 	}

// 	return false
// }

// Combine combines multiple signatures together into a single signature.
func (cache *cache) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	// we don't cache the result of this operation, because it is not guaranteed to be valid.
	return cache.impl.Combine(signatures...)
}
