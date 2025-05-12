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

// Globals stores runtime configuration settings.
type Globals struct {
	id         hotstuff.ID
	privateKey hotstuff.PrivateKey

	shouldUseAggQC        bool
	shouldVerifyVotesSync bool

	sharedRandomSeed   int64
	connectionMetadata map[string]string

	tree          *tree.Tree
	shouldUseTree bool
	useKauri      bool
}

func NewGlobals(id hotstuff.ID, pk hotstuff.PrivateKey, opts ...GlobalsOption) *Globals {
	g := &Globals{
		id:                 id,
		privateKey:         pk,
		connectionMetadata: make(map[string]string),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *Globals) ShouldUseTree() bool {
	return g.shouldUseTree
}

// ID returns the ID.
func (g *Globals) ID() hotstuff.ID {
	return g.id
}

// PrivateKey returns the private key.
func (g *Globals) PrivateKey() hotstuff.PrivateKey {
	return g.privateKey
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
// TODO(AlanRostem): Find out a way to avoid setting this inside a module
func (g *Globals) SetShouldUseAggQC() {
	g.shouldUseAggQC = true
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (g *Globals) ShouldUseAggQC() bool {
	return g.shouldUseAggQC
}

// ShouldVerifyVotesSync returns true if votes should be verified synchronously.
// Enabling this should make the voting machine process votes synchronously.
func (g *Globals) ShouldVerifyVotesSync() bool {
	return g.shouldVerifyVotesSync
}

// SharedRandomSeed returns a random number that is shared between all replicas.
func (g *Globals) SharedRandomSeed() int64 {
	return g.sharedRandomSeed
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (g *Globals) ConnectionMetadata() map[string]string {
	return g.connectionMetadata
}

// SetSharedRandomSeed sets the shared random seed.
func (g *Globals) SetSharedRandomSeed(seed int64) {
	g.sharedRandomSeed = seed
}

// ShouldEnableKauri returns true if Kauri will be integrated to the consensus logic and
// Kauri-specific RPC methods are getting enabled.
func (g *Globals) ShouldEnableKauri() bool {
	return g.useKauri
}

// Tree returns the tree configuration.
func (g *Globals) Tree() *tree.Tree {
	return g.tree
}

// WithMetaData sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func (g *Globals) SetConnectionMetadata(key string, value string) {
	g.connectionMetadata[key] = value
}
