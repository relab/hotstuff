package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

const NameClientLatency = "client-latency"

// clientLatency processes LatencyMeasurementEvents, and writes LatencyMeasurements to the metrics logger.
type clientLatency struct {
	metricsLogger Logger
	id            hotstuff.ID

	wf Welford
}

func enableClientLatency(
	el *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	lr := &clientLatency{
		id:            id,
		metricsLogger: metricsLogger,
	}
	eventloop.Register(el, func(event client.LatencyMeasurementEvent) {
		lr.addLatency(event.Latency)
	})
	eventloop.Register(el, func(tickEvent types.TickEvent) {
		lr.tick(tickEvent)
	}, eventloop.Prioritize())
}

// AddLatency adds a latency data point to the current measurement.
func (lr *clientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *clientLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewClientEvent(uint32(lr.id), time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
