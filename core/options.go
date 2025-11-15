package core

import "github.com/relab/hotstuff/internal/tree"

type RuntimeOption func(*RuntimeConfig)

// WithSyncVerification forces synchronous verification of incoming votes at the leader.
// The default is to verify votes concurrently. Forcing synchronous vote verification
// can make it easier to debug.
func WithSyncVerification() RuntimeOption {
	return func(rc *RuntimeConfig) {
		rc.syncVoteVerification = true
	}
}

// WithKauriTree adds a tree to the config to be used by a tree-based leader scheme in
// the Kauri protocol.
func WithKauriTree(t *tree.Tree) RuntimeOption {
	return func(g *RuntimeConfig) {
		g.tree = t
	}
}

// WithSharedRandomSeed adds a seed shared among replicas.
// Default: 0
func WithSharedRandomSeed(seed int64) RuntimeOption {
	return func(g *RuntimeConfig) {
		g.sharedRandomSeed = seed
	}
}

// WithAggregateQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-HotStuff: https://arxiv.org/abs/2010.11454
func WithAggregateQC() RuntimeOption {
	return func(g *RuntimeConfig) {
		g.aggQC = true
	}
}

// WithCache specifies the cache size for crypto operations. This option causes
// the Crypto implementation to be wrapped in a caching layer that caches the
// results of recent crypto operations, avoiding repeated computations.
func WithCache(size uint) RuntimeOption {
	return func(g *RuntimeConfig) {
		g.cacheSize = size
	}
}
