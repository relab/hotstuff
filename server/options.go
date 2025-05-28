package server

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

type serverOptions struct {
	id            hotstuff.ID
	latencyMatrix latency.Matrix
	gorumsSrvOpts []gorums.ServerOption
}

// ServerOption is a function for configuring the Server.
type ServerOption func(*serverOptions)

// WithLatencies sets the locations assigned to the replicas and
// constructs the corresponding latency matrix.
func WithLatencies(id hotstuff.ID, locations []string) ServerOption {
	return func(opts *serverOptions) {
		opts.id = id
		opts.latencyMatrix = latency.MatrixFrom(locations)
	}
}

// WithGorumsServerOptions sets the gorums server options.
func WithGorumsServerOptions(opts ...gorums.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.gorumsSrvOpts = append(o.gorumsSrvOpts, opts...)
	}
}
