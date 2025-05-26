package metrics

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// This file implements a method to enable metrics

// Enable injects necessary dependencies to enable the metrics the user wants to measure.
func Enable(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	id hotstuff.ID,
	measurementInterval time.Duration,
	metricNames ...string) error {
	if len(metricNames) == 0 {
		return fmt.Errorf("no metric names provided")
	}
	for _, name := range metricNames {
		switch name {
		case NameClientLatency:
			enableClientLatency(eventLoop, metricsLogger, id)
		case NameViewTimeouts:
			enableViewTimeouts(eventLoop, metricsLogger, id)
		case NameThroughput:
			enableThroughput(eventLoop, metricsLogger, id)
		case NameConsensusLatency:
			enableConsensusLatency(eventLoop, metricsLogger, id)
		default:
			return fmt.Errorf("invalid metric: %s", name)
		}
		logger.Infof("metric enabled: %s", name)
	}
	addTicker(eventLoop, measurementInterval)
	return nil
}
