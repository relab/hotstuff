package globals

import "github.com/relab/hotstuff/internal/tree"

type Option func(*Globals)

// WithTree adds a tree to the globals to be used by a tree-leader.
func WithTree(t *tree.Tree) Option {
	return func(g *Globals) {
		g.shouldUseTree = true
		g.tree = t
	}
}

// WithKauri enables Kauri protocol.
// NOTE: Requires WithTree. If not used then it panics.
func WithKauri() Option {
	return func(g *Globals) {
		if !g.shouldUseTree {
			panic("no tree was set")
		}
		g.useKauri = true
	}
}

// WithSharedRandomSeed adds a seed shared among replicas.
func WithSharedRandomSeed(seed int64) Option {
	return func(g *Globals) {
		g.sharedRandomSeed = seed
	}
}
