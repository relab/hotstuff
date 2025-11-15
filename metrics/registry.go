package metrics

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// Enable enables logging of the specified client and replica metrics.
// If id is a client.ID, client metrics will be enabled.
// If id is a hotstuff.ID, replica metrics will be enabled.
// The measurementInterval specifies how often measurements are logged.
// Valid metric names are defined as constants in their respective metric files.
func Enable[T client.ID | hotstuff.ID](
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	id T,
	measurementInterval time.Duration,
	metricNames ...string,
) error {
	if len(metricNames) == 0 {
		return fmt.Errorf("no metric names provided")
	}
	enabledMetrics := []string{}
	for _, name := range metricNames {
		switch name {
		case NameClientLatency:
			if clientID, ok := any(id).(client.ID); ok {
				enableClientLatency(eventLoop, metricsLogger, clientID)
				enabledMetrics = append(enabledMetrics, NameClientLatency)
			}
		case NameViewTimeouts:
			if replicaID, ok := any(id).(hotstuff.ID); ok {
				enableViewTimeouts(eventLoop, metricsLogger, replicaID)
				enabledMetrics = append(enabledMetrics, NameViewTimeouts)
			}
		case NameThroughput:
			if replicaID, ok := any(id).(hotstuff.ID); ok {
				enableThroughput(eventLoop, metricsLogger, replicaID)
				enabledMetrics = append(enabledMetrics, NameThroughput)
			}
		case NameConsensusLatency:
			if replicaID, ok := any(id).(hotstuff.ID); ok {
				enableConsensusLatency(eventLoop, metricsLogger, replicaID)
				enabledMetrics = append(enabledMetrics, NameConsensusLatency)
			}
		default:
			return fmt.Errorf("invalid metric: %s", name)
		}
	}
	logger.Infof("Metrics enabled: %v", enabledMetrics)
	addTicker(eventLoop, measurementInterval)
	return nil
}
