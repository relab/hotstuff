package metrics

import (
	"time"

	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/metrics/types"
)

const NameClientLatency = "client-latency"

// clientLatency processes LatencyMeasurementEvents, and writes LatencyMeasurements to the metrics logger.
type clientLatency struct {
	metricsLogger Logger
	opts          *core.Options

	wf Welford
}

func enableClientLatency(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	opts *core.Options,
	metricsLogger Logger,
) {
	lr := &clientLatency{
		opts:          opts,
		metricsLogger: metricsLogger,
	}

	eventLoop.RegisterHandler(client.LatencyMeasurementEvent{}, func(event any) {
		latencyEvent := event.(client.LatencyMeasurementEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		lr.tick(event.(types.TickEvent))
	}, eventloop.Prioritize())

	logger.Info("Client Latency metric enabled")
}

// AddLatency adds a latency data point to the current measurement.
func (lr *clientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *clientLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewClientEvent(uint32(lr.opts.ID()), time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
