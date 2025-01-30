package metrics

import (
	"sync"
	"time"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// This file implements a registry for metrics.
// The purpose of the registry is to make it possible to instantiate multiple metrics based on their names only.
// We distinguish between client metrics and replica metrics, but a single metric could work on both.

var (
	registryMut    sync.Mutex
	clientMetrics  = map[string]func() any{}
	replicaMetrics = map[string]func() any{}
)

func InitMeasurements(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	opts *core.Options,
	measurementInterval time.Duration,
	names ...string) {
	for _, name := range names {
		switch name {
		case NameClientLatency:
			NewClientLatency(eventLoop, logger, opts, metricsLogger)
		case NameViewTimeouts:
			NewViewTimeouts(eventLoop, logger, metricsLogger, opts)
		case NameThroughput:
			NewThroughput(eventLoop, logger, metricsLogger, opts)
		}
	}
	NewTicker(eventLoop, measurementInterval)
}

// GetClientMetrics constructs a new instance of each named metric.
func GetClientMetrics(names ...string) (metrics []any) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := clientMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}

// GetReplicaMetrics constructs a new instance of each named metric.
func GetReplicaMetrics(names ...string) (metrics []any) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := replicaMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}
