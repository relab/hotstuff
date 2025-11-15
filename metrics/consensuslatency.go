package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

const NameConsensusLatency = "consensus-latency"

// consensusLatency measures the latency of consensus decisions.
type consensusLatency struct {
	metricsLogger Logger
	id            hotstuff.ID
	wf            Welford
}

// enableConsensusLatency enables consensus latency measurement.
func enableConsensusLatency(
	el *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	lr := consensusLatency{
		metricsLogger: metricsLogger,
		id:            id,
	}
	eventloop.Register(el, func(event hotstuff.ConsensusLatencyEvent) {
		lr.addLatency(event.Latency)
	})
	eventloop.Register(el, func(tickEvent types.TickEvent) {
		lr.tick(tickEvent)
	}, eventloop.Prioritize())
}

// addLatency adds a latency data point to the current measurement.
func (lr *consensusLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

// tick logs the current latency measurement to the metrics logger.
func (lr *consensusLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewReplicaEvent(lr.id, time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
