package metrics

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// This file implements a method to enable metrics

// Enable injects necessary dependencies to enable the metrics the user wants to measure.
func Enable(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	config *core.RuntimeConfig,
	measurementInterval time.Duration,
	metricNames ...string) error {
	if len(metricNames) == 0 {
		return fmt.Errorf("no metric names provided")
	}
	for _, name := range metricNames {
		switch name {
		case NameClientLatency:
			enableClientLatency(eventLoop, logger, metricsLogger, config)
		case NameViewTimeouts:
			enableViewTimeouts(eventLoop, logger, metricsLogger, config)
		case NameThroughput:
			enableThroughput(eventLoop, logger, metricsLogger, config)
		default:
			return fmt.Errorf("invalid metric: %s", name)
		}
	}
	addTicker(eventLoop, measurementInterval)
	return nil
}
