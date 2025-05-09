package server

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

type backendOptions struct {
	id            hotstuff.ID
	latencyMatrix latency.Matrix
	gorumsSrvOpts []gorums.ServerOption
	registerKauri bool
}

// ServerOptions is a function for configuring the Server.
type ServerOptions func(*backendOptions)

// WithKauri registers a service implementation for Gorums which allows sending hotstuff.ContributionRecvEvent.
func WithKauri() ServerOptions {
	return func(opts *backendOptions) {
		opts.registerKauri = true
	}
}

// WithLatencies sets the locations assigned to the replicas and
// constructs the corresponding latency matrix.
func WithLatencies(id hotstuff.ID, locations []string) ServerOptions {
	return func(opts *backendOptions) {
		opts.id = id
		opts.latencyMatrix = latency.MatrixFrom(locations)
	}
}

// WithGorumsServerOptions sets the gorums server options.
func WithGorumsServerOptions(opts ...gorums.ServerOption) ServerOptions {
	return func(o *backendOptions) {
		o.gorumsSrvOpts = append(o.gorumsSrvOpts, opts...)
	}
}
