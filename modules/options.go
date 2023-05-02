package modules

import (
	"sync"
	"sync/atomic"

	"github.com/relab/hotstuff"
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
//	opts.Get(a.MyOption)
var nextID OptionID

// NewOption returns a new option ID.
func NewOption() OptionID {
	return OptionID(atomic.AddUint64((*uint64)(&nextID), 1))
}

// Options stores runtime configuration settings.
type Options struct {
	mut     sync.Mutex
	options []any

	id         hotstuff.ID
	privateKey hotstuff.PrivateKey

	shouldUseAggQC        bool
	shouldUseHandel       bool
	shouldVerifyVotesSync bool

	sharedRandomSeed   int64
	connectionMetadata map[string]string
}

func (opts *Options) ensureSpace(id OptionID) {
	if int(id) >= len(opts.options) {
		newOpts := make([]any, id+1)
		copy(newOpts, opts.options)
		opts.options = newOpts
	}
}

// Get returns the value associated with the given option ID.
func (opts *Options) Get(id OptionID) any {
	opts.mut.Lock()
	defer opts.mut.Unlock()
	if len(opts.options) <= int(id) {
		return nil
	}
	return opts.options[id]
}

// Set sets the value of the given option ID.
func (opts *Options) Set(id OptionID, value any) {
	opts.mut.Lock()
	defer opts.mut.Unlock()
	opts.ensureSpace(id)
	opts.options[id] = value
}

// ID returns the ID.
func (opts *Options) ID() hotstuff.ID {
	return opts.id
}

// PrivateKey returns the private key.
func (opts *Options) PrivateKey() hotstuff.PrivateKey {
	return opts.privateKey
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (opts *Options) ShouldUseAggQC() bool {
	return opts.shouldUseAggQC
}

// ShouldUseHandel returns true if the Handel signature aggregation protocol should be used.
func (opts *Options) ShouldUseHandel() bool {
	return opts.shouldUseHandel
}

// ShouldVerifyVotesSync returns true if votes should be verified synchronously.
// Enabling this should make the voting machine process votes synchronously.
func (opts *Options) ShouldVerifyVotesSync() bool {
	return opts.shouldVerifyVotesSync
}

// SharedRandomSeed returns a random number that is shared between all replicas.
func (opts *Options) SharedRandomSeed() int64 {
	return opts.sharedRandomSeed
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (opts *Options) ConnectionMetadata() map[string]string {
	return opts.connectionMetadata
}

// SetShouldUseAggQC sets the ShouldUseAggQC setting to true.
func (opts *Options) SetShouldUseAggQC() {
	opts.shouldUseAggQC = true
}

// SetShouldUseHandel sets the ShouldUseHandel setting to true.
func (opts *Options) SetShouldUseHandel() {
	opts.shouldUseHandel = true
}

// SetShouldVerifyVotesSync sets the ShouldVerifyVotesSync setting to true.
func (opts *Options) SetShouldVerifyVotesSync() {
	opts.shouldVerifyVotesSync = true
}

// SetSharedRandomSeed sets the shared random seed.
func (opts *Options) SetSharedRandomSeed(seed int64) {
	opts.sharedRandomSeed = seed
}

// SetConnectionMetadata sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func (opts *Options) SetConnectionMetadata(key string, value string) {
	opts.connectionMetadata[key] = value
}
