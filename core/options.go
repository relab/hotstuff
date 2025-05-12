package core

import "github.com/relab/hotstuff/internal/tree"

type GlobalOption func(*Globals)

func WithTree(t *tree.Tree) GlobalOption {
	return func(g *Globals) {
		g.shouldUseTree = true
		g.tree = t
	}
}

// WithKauri enables Kauri protocol. If WithTree was not used then it panics.
func WithKauri() GlobalOption {
	return func(g *Globals) {
		if !g.shouldUseTree {
			panic("no tree was set")
		}
		g.useKauri = true
	}
}

func WithSharedRandomSeed(seed int64) GlobalOption {
	return func(g *Globals) {
		g.sharedRandomSeed = seed
	}
}
