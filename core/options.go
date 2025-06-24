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
