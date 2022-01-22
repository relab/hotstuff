package crypto

import (
	"container/list"
	"crypto/sha256"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type cache struct {
	impl        consensus.CryptoImpl
	mut         sync.Mutex
	capacity    int
	entries     map[consensus.Hash]*list.Element
	accessOrder list.List
}

// NewCache returns a new Crypto implementation that caches the results of the operations of the given CryptoImpl
// implementation.
func NewCache(impl consensus.CryptoImpl, capacity int) consensus.Crypto {
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

func (cache *cache) insert(key consensus.Hash) {
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

func (cache *cache) check(key consensus.Hash) bool {
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
	key := cache.accessOrder.Remove(cache.accessOrder.Back()).(consensus.Hash)
	delete(cache.entries, key)
}

// Sign signs a hash.
func (cache *cache) Sign(hash consensus.Hash) (sig consensus.Signature, err error) {
	sig, err = cache.impl.Sign(hash)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	cache.insert(key)
	return sig, nil
}

// Verify verifies a signature given a hash.
func (cache *cache) Verify(sig consensus.Signature, hash consensus.Hash) bool {
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
func (cache *cache) CreateThresholdSignature(partialSignatures []consensus.Signature, hash consensus.Hash) (sig consensus.ThresholdSignature, err error) {
	sig, err = cache.impl.CreateThresholdSignature(partialSignatures, hash)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(append(hash[:], sig.ToBytes()...))
	cache.insert(key)
	return sig, nil
}

// VerifyThresholdSignature verifies a threshold signature.
func (cache *cache) VerifyThresholdSignature(signature consensus.ThresholdSignature, hash consensus.Hash) bool {
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

// CreateThresholdSignatureForMessageSet creates a threshold signature where each partial signature has signed a
// different message hash.
func (cache *cache) CreateThresholdSignatureForMessageSet(partialSignatures []consensus.Signature, hashes map[hotstuff.ID]consensus.Hash) (consensus.ThresholdSignature, error) {
	signature, err := cache.impl.CreateThresholdSignatureForMessageSet(partialSignatures, hashes)
	if err != nil {
		return nil, err
	}
	var key consensus.Hash
	hash := sha256.New()
	for _, h := range hashes {
		hash.Write(h[:])
	}
	hash.Write(signature.ToBytes())
	hash.Sum(key[:0])
	cache.insert(key)
	return signature, nil
}

// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
func (cache *cache) VerifyThresholdSignatureForMessageSet(signature consensus.ThresholdSignature, hashes map[hotstuff.ID]consensus.Hash) bool {
	if signature == nil {
		return false
	}
	var key consensus.Hash
	hash := sha256.New()
	for _, h := range hashes {
		hash.Write(h[:])
	}
	hash.Write(signature.ToBytes())
	hash.Sum(key[:0])
	if cache.check(key) {
		return true
	}
	if cache.impl.VerifyThresholdSignatureForMessageSet(signature, hashes) {
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
func (cache *cache) Combine(signatures ...interface{}) consensus.ThresholdSignature {
	// we don't cache the result of this operation, because it is not guaranteed to be valid.
	return cache.impl.Combine(signatures...)
}
