package core

import "github.com/relab/hotstuff/internal/tree"

type GlobalsOption func(*Globals)

func WithKauri(kauriTree *tree.Tree) GlobalsOption {
	return func(g *Globals) {
		g.shouldUseTree = true
		g.tree = *kauriTree
	}
}

func WithAggQCUsage() GlobalsOption {
	return func(g *Globals) {
		g.shouldUseAggQC = true
	}
}

func WithVotesSyncVerification() GlobalsOption {
	return func(g *Globals) {
		g.shouldVerifyVotesSync = true
	}
}

// WithMetaData sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func WithMetaData(key string, value string) GlobalsOption {
	return func(g *Globals) {
		g.connectionMetadata[key] = value
	}
}
