package crypto

import (
	"container/list"
	"crypto/sha256"
	"sync"

	"github.com/relab/hotstuff/consensus"
)

type cache struct {
	impl        consensus.CryptoBase
	mut         sync.Mutex
	capacity    int
	entries     map[consensus.Hash]*list.Element
	accessOrder list.List
}

// NewCache returns a new Crypto implementation that caches the results of the operations of the given CryptoImpl
// implementation.
func NewCache(impl consensus.CryptoBase, capacity int) consensus.Crypto {
	return New(&cache{
		impl:     impl,
		capacity: capacity,
		entries:  make(map[consensus.Hash]*list.Element, capacity),
	})
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (cache *cache) InitConsensusModule(mods *consensus.Modules, cfg *consensus.OptionsBuilder) {
	if mod, ok := cache.impl.(consensus.Module); ok {
		mod.InitConsensusModule(mods, cfg)
	}
}

func (cache *cache) insert(hash consensus.Hash) {
	cache.mut.Lock()
	defer cache.mut.Unlock()
	elem, ok := cache.entries[hash]
	if ok {
		cache.accessOrder.MoveToFront(elem)
		return
	}
	cache.evict()
	elem = cache.accessOrder.PushFront(hash)
	cache.entries[hash] = elem
}

func (cache *cache) check(hash consensus.Hash) bool {
	cache.mut.Lock()
	defer cache.mut.Unlock()
	elem, ok := cache.entries[hash]
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
	key := cache.accessOrder.Remove(cache.accessOrder.Back()).(consensus.Hash)
	delete(cache.entries, key)
}

// Sign signs a hash.
func (cache *cache) Sign(hash consensus.Hash) (sig consensus.QuorumSignature, err error) {
	sig, err = cache.impl.Sign(hash)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	cache.insert(key)
	return sig, nil
}

// Verify verifies a signature given a hash.
func (cache *cache) Verify(sig consensus.QuorumSignature, options ...consensus.VerifyOption) bool {
	if sig == nil {
		return false
	}

	var opts consensus.VerifyOptions
	for _, opt := range options {
		opt(&opts)
	}

	if sig.Participants().Len() < opts.Threshold {
		return false
	}

	var key consensus.Hash
	hash := sha256.New()
	if opts.UseHashMap {
		for _, h := range opts.HashMap {
			hash.Write(h[:])
		}
	} else {
		hash.Write(opts.Hash[:])
	}
	hash.Write(sig.ToBytes())
	hash.Sum(key[:0])

	if cache.check(key) {
		return true
	}

	if cache.impl.Verify(sig, options...) {
		cache.insert(key)
		return true
	}

	return false
}

// Combine combines multiple signatures into a single threshold signature.
// Arguments can be singular signatures or threshold signatures.
//
// As opposed to the CreateThresholdSignature methods,
// this method does not check whether the resulting
// signature meets the quorum size.
func (cache *cache) Combine(signatures ...consensus.QuorumSignature) (consensus.QuorumSignature, error) {
	// we don't cache the result of this operation, because it is not guaranteed to be valid.
	sig, err := cache.impl.Combine(signatures...)
	return sig, err
}
