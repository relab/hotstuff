package crypto

import (
	"container/list"
	"crypto/sha256"
	"sync"

	"github.com/relab/hotstuff"
)

type cache struct {
	impl        hotstuff.CryptoImpl
	mut         sync.Mutex
	capacity    int
	entries     map[hotstuff.Hash]*list.Element
	accessOrder list.List
}

// NewCache returns a new Crypto implementation that caches the results of the operations of the given CryptoImpl
// implementation.
func NewCache(impl hotstuff.CryptoImpl, capacity int) hotstuff.Crypto {
	return New(&cache{
		impl:     impl,
		capacity: capacity,
		entries:  make(map[hotstuff.Hash]*list.Element, capacity),
	})
}

// InitModule gives the module a reference to the HotStuff object.
func (cache *cache) InitModule(hs *hotstuff.HotStuff) {
	if mod, ok := cache.impl.(hotstuff.Module); ok {
		mod.InitModule(hs)
	}
}

func (cache *cache) insert(key hotstuff.Hash) {
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

func (cache *cache) check(key hotstuff.Hash) bool {
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
	key := cache.accessOrder.Remove(cache.accessOrder.Back()).(hotstuff.Hash)
	delete(cache.entries, key)
}

// Sign signs a hash.
func (cache *cache) Sign(hash hotstuff.Hash) (sig hotstuff.Signature, err error) {
	sig, err = cache.impl.Sign(hash)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	cache.insert(key)
	return sig, nil
}

// Verify verifies a signature given a hash.
func (cache *cache) Verify(sig hotstuff.Signature, hash hotstuff.Hash) bool {
	if sig == nil {
		return false
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	if cache.check(key) {
		return true
	}
	if cache.impl.Verify(sig, hash) {
		cache.insert(key)
		return true
	}
	return false
}

// CreateThresholdSignature creates a threshold signature from the given partial signatures.
func (cache *cache) CreateThresholdSignature(partialSignatures []hotstuff.Signature, hash hotstuff.Hash) (sig hotstuff.ThresholdSignature, err error) {
	sig, err = cache.impl.CreateThresholdSignature(partialSignatures, hash)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	cache.insert(key)
	return sig, nil
}

// VerifyThresholdSignature verifies a threshold signature.
func (cache *cache) VerifyThresholdSignature(signature hotstuff.ThresholdSignature, hash hotstuff.Hash) bool {
	if signature == nil {
		return false
	}
	key := sha256.Sum256(append(hash[:], signature.ToBytes()...))
	if cache.check(key) {
		return true
	}
	if cache.impl.VerifyThresholdSignature(signature, hash) {
		cache.insert(key)
		return true
	}
	return false
}
