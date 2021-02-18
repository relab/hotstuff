package ecdsa

import (
	"container/list"
	"sync"
	"sync/atomic"

	"github.com/relab/hotstuff"
)

type signatureCache struct {
	ecdsaCrypto

	mut      sync.Mutex
	cache    map[string]*list.Element
	capacity int
	order    list.List
}

// NewWithCache returns a new Signer and a new Verifier that use caching to speed up verification
func NewWithCache(cfg hotstuff.Config, capacity int) (hotstuff.Signer, hotstuff.Verifier) {
	cache := &signatureCache{
		ecdsaCrypto: ecdsaCrypto{cfg},
		cache:       make(map[string]*list.Element, capacity),
		capacity:    capacity,
	}
	return cache, cache
}

func (c *signatureCache) dropOldest() {
	elem := c.order.Back()
	delete(c.cache, elem.Value.(string))
	c.order.Remove(elem)
}

func (c *signatureCache) insert(key string) {
	if elem, ok := c.cache[key]; ok {
		c.order.MoveToFront(elem)
		return
	}

	if len(c.cache)+1 > c.capacity {
		c.dropOldest()
	}

	elem := c.order.PushFront(key)
	c.cache[key] = elem
}

func (c *signatureCache) check(key string) bool {
	elem, ok := c.cache[key]
	if !ok {
		return false
	}
	c.order.MoveToFront(elem)
	return true
}

// Sign signs a single block and returns the partial certificate.
func (c *signatureCache) Sign(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	signature, err := c.ecdsaCrypto.Sign(block)
	if err != nil {
		return nil, err
	}
	k := string(signature.ToBytes())
	c.mut.Lock()
	c.insert(k)
	c.mut.Unlock()
	return signature, nil
}

// VerifyPartialCert verifies a single partial certificate.
func (c *signatureCache) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	k := string(cert.Signature().ToBytes())

	c.mut.Lock()
	if c.check(k) {
		c.mut.Unlock()
		return true
	}
	c.mut.Unlock()

	if !c.ecdsaCrypto.VerifyPartialCert(cert) {
		return false
	}

	c.mut.Lock()
	c.insert(k)
	c.mut.Unlock()

	return true
}

// VerifyQuorumCert verifies a quorum certificate.
func (c *signatureCache) VerifyQuorumCert(qc hotstuff.QuorumCert) bool {
	// If QC was created for genesis, then skip verification.
	if qc.BlockHash() == hotstuff.GetGenesis().Hash() {
		return true
	}

	ecdsaQC := qc.(*QuorumCert)
	if len(ecdsaQC.signatures) < c.ecdsaCrypto.cfg.QuorumSize() {
		return false
	}

	var wg sync.WaitGroup
	var numValid uint32

	// first check if any signatures are cache
	c.mut.Lock()
	for _, sig := range ecdsaQC.signatures {
		k := string(sig.ToBytes())
		if c.check(k) {
			numValid++
			continue
		}
		// on cache miss, we start a goroutine to verify the signature
		wg.Add(1)
		go func(sig *Signature) {
			if c.ecdsaCrypto.verifySignature(sig, ecdsaQC.hash) {
				atomic.AddUint32(&numValid, 1)
				c.mut.Lock()
				c.insert(string(sig.ToBytes()))
				c.mut.Unlock()
			}
			wg.Done()
		}(sig)
	}
	c.mut.Unlock()

	wg.Wait()
	return numValid >= uint32(c.ecdsaCrypto.cfg.QuorumSize())
}
