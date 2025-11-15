package metrics

import (
	"time"

	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

const NameClientLatency = "client-latency"

// clientLatency measures the latency of client requests.
type clientLatency struct {
	metricsLogger Logger
	id            client.ID
	wf            Welford
}

// enableClientLatency enables client latency measurement.
func enableClientLatency(
	el *eventloop.EventLoop,
	metricsLogger Logger,
	id client.ID,
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

// addLatency adds a latency data point to the current measurement.
func (lr *clientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

// tick logs the current latency measurement to the metrics logger.
func (lr *clientLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewClientEvent(lr.id, time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
