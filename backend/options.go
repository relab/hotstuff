package backend

import (
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
)

type backendOptions struct {
	location          string
	locationInfo      map[hotstuff.ID]string
	locationLatencies map[string]time.Duration
	gorumsSrvOpts     []gorums.ServerOption
}

// ServerOptions is a function for configuring the Server.
type ServerOptions func(*backendOptions)

// WithLatencyInfo sets the location of the replica and the latency matrix.
func WithLatencyInfo(id hotstuff.ID, locationInfo map[hotstuff.ID]string) ServerOptions {
	return func(opts *backendOptions) {
		location, ok := locationInfo[id]
		if !ok {
			opts.location = hotstuff.DefaultLocation
			return
		}
		opts.location = location
		opts.locationInfo = locationInfo
		locationLatencies, ok := latencies[location]
		if !ok {
			opts.location = hotstuff.DefaultLocation
			return
		}
		opts.locationLatencies = locationLatencies
	}
}

// WithGorumsServerOptions sets the gorums server options.
func WithGorumsServerOptions(opts ...gorums.ServerOption) ServerOptions {
	return func(o *backendOptions) {
		o.gorumsSrvOpts = append(o.gorumsSrvOpts, opts...)
	}
}
