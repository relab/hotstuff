package core

import "github.com/relab/hotstuff/internal/tree"

type GlobalsOption func(*Globals)

func WithKauri(kauriTree *tree.Tree) GlobalsOption {
	return func(g *Globals) {
		g.shouldUseTree = true
		g.useKauri = true
		g.tree = *kauriTree
	}
}

func WithSharedRandomSeed(seed int64) GlobalsOption {
	return func(g *Globals) {
		g.sharedRandomSeed = seed
	}
}
