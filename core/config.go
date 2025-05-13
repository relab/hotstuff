package core

import (
	"sync/atomic"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/tree"
)

// OptionID is the ID of an option.
type OptionID uint64

// nextID stores the ID for the next option created by NewOptions.
// We want this to be global so that a package can export the option IDs it uses.
// For example,
//
//	package a
//	var MyOption = modules.NewOption()
//
//	package b
//
//	import "a"
//
//	g.Get(a.MyOption)
var nextID OptionID

// NewOption returns a new option ID.
func NewOption() OptionID {
	return OptionID(atomic.AddUint64((*uint64)(&nextID), 1))
}

// RuntimeConfig stores runtime configuration settings.
type RuntimeConfig struct {
	id         hotstuff.ID
	privateKey hotstuff.PrivateKey

	aggQCEnabled                bool
	syncVoteVerificationEnabled bool

	sharedRandomSeed int64

	connectionMetadata map[string]string
	replicas           map[hotstuff.ID]*hotstuff.ReplicaInfo

	treeEnabled  bool
	kauriEnabled bool
	tree         *tree.Tree
}

func NewRuntimeConfig(id hotstuff.ID, pk hotstuff.PrivateKey, opts ...Option) *RuntimeConfig {
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

// EnableAggregateQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
// TODO(AlanRostem): Find out a way to avoid setting this inside a module
func (g *RuntimeConfig) EnableAggregateQC() {
	g.aggQCEnabled = true
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
// Kauri-specific RPC methods are getting enabled.
func (g *RuntimeConfig) KauriEnabled() bool {
	return g.kauriEnabled
}

// HasTree returns true if a tree was set.
func (g *RuntimeConfig) HasTree() bool {
	return g.treeEnabled
}

// Tree returns the tree configuration.
func (g *RuntimeConfig) Tree() *tree.Tree {
	return g.tree
}
