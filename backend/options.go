package backend

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

type backendOptions struct {
	id            hotstuff.ID
	latencyMatrix latency.Matrix
	gorumsSrvOpts []gorums.ServerOption
}

// ServerOptions is a function for configuring the Server.
type ServerOptions func(*backendOptions)

// WithLocations sets the locations assigned to the replicas and
// constructs the corresponding latency matrix.
func WithLocations(id hotstuff.ID, locations []string) ServerOptions {
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
