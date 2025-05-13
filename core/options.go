package core

import "github.com/relab/hotstuff/internal/tree"

type RuntimeOption func(*RuntimeConfig)

// WithTree adds a tree to the config to be used by a tree-leader.
func WithTree(t *tree.Tree) RuntimeOption {
	return func(g *RuntimeConfig) {
		g.treeEnabled = true
		g.tree = t
	}
}

// WithKauri enables Kauri protocol.
// NOTE: Requires WithTree. If not used then it panics.
func WithKauri() RuntimeOption {
	return func(g *RuntimeConfig) {
		if !g.treeEnabled {
			panic("no tree was set")
		}
		g.kauriEnabled = true
	}
}

// WithSharedRandomSeed adds a seed shared among replicas.
func WithSharedRandomSeed(seed int64) RuntimeOption {
	return func(g *RuntimeConfig) {
		g.sharedRandomSeed = seed
	}
}

// WithAggregateQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func WithAggregateQC() RuntimeOption {
	return func(g *RuntimeConfig) {
		g.aggQCEnabled = true
	}
}
