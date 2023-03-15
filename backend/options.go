package backend

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
)

type backendOptions struct {
	location          string
	locationInfo      map[uint32]string
	locationLatencies map[string]float64
	gorumsSrvOpts     []gorums.ServerOption
}

type ServerOptions func(*backendOptions)

func WithLatencyInfo(id hotstuff.ID, locationInfo map[uint32]string) ServerOptions {
	return func(opts *backendOptions) {
		location, ok := locationInfo[uint32(id)]
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

func WithGorumsServerOptions(opts ...gorums.ServerOption) ServerOptions {
	return func(o *backendOptions) {
		o.gorumsSrvOpts = append(o.gorumsSrvOpts, opts...)
	}
}
