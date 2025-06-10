package core

import "github.com/relab/hotstuff/internal/tree"

type RuntimeOption func(*RuntimeConfig)

// WithSyncVoteVerification enables synchronous verification of incoming votes at the leader.
func WithSyncVoteVerification() RuntimeOption {
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
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func WithAggregateQC() RuntimeOption {
	return func(g *RuntimeConfig) {
		g.aggQC = true
	}
}
