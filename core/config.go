package core

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/tree"
)

// RuntimeConfig stores runtime configuration settings.
type RuntimeConfig struct {
	id         hotstuff.ID
	privateKey hotstuff.PrivateKey

	aggQCEnabled                bool
	kauriEnabled                bool
	syncVoteVerificationEnabled bool

	connectionMetadata map[string]string
	replicas           map[hotstuff.ID]*hotstuff.ReplicaInfo

	sharedRandomSeed int64

	tree *tree.Tree
}

func NewRuntimeConfig(id hotstuff.ID, pk hotstuff.PrivateKey, opts ...RuntimeOption) *RuntimeConfig {
	g := &RuntimeConfig{
		id:                 id,
		privateKey:         pk,
		connectionMetadata: make(map[string]string),
		replicas:           make(map[hotstuff.ID]*hotstuff.ReplicaInfo),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// ID returns the ID.
func (g *RuntimeConfig) ID() hotstuff.ID {
	return g.id
}

// PrivateKey returns the private key.
func (g *RuntimeConfig) PrivateKey() hotstuff.PrivateKey {
	return g.privateKey
}

// HasAggregateQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (g *RuntimeConfig) HasAggregateQC() bool {
	return g.aggQCEnabled
}

// SyncVoteVerification returns true if votes should be verified synchronously.
// Enabling this should make the voting machine process votes synchronously.
func (g *RuntimeConfig) SyncVoteVerification() bool {
	return g.syncVoteVerificationEnabled
}

// SharedRandomSeed returns a random number that is shared between all replicas.
func (g *RuntimeConfig) SharedRandomSeed() int64 {
	return g.sharedRandomSeed
}

// KauriEnabled returns true if Kauri will be integrated to the consensus logic and
// Kauri-specific RPC methods should be registered.
func (g *RuntimeConfig) KauriEnabled() bool {
	return g.kauriEnabled
}

// HasTree returns true if a tree was set for the tree-based leader scheme.
func (g *RuntimeConfig) HasTree() bool {
	return g.tree != nil
}

// Tree returns the tree configuration for the tree-based leader scheme.
func (g *RuntimeConfig) Tree() *tree.Tree {
	return g.tree
}
