package metrics

import (
	"time"

	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterClientMetric("client-latency", func() any {
		return &ClientLatency{}
	})
}

// ClientLatency processes LatencyMeasurementEvents, and writes LatencyMeasurements to the metrics logger.
type ClientLatency struct {
	metricsLogger Logger
	opts          *modules.Options

	wf Welford
}

// InitModule gives the module access to the other modules.
func (lr *ClientLatency) InitModule(mods *modules.Core) {
	var (
		eventLoop *eventloop.EventLoop
		logger    logging.Logger
	)

	mods.Get(
		&lr.metricsLogger,
		&lr.opts,
		&eventLoop,
		&logger,
	)

	eventLoop.RegisterHandler(client.LatencyMeasurementEvent{}, func(event any) {
		latencyEvent := event.(client.LatencyMeasurementEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		lr.tick(event.(types.TickEvent))
	})

	logger.Info("Client Latency metric enabled")
}

// AddLatency adds a latency data point to the current measurement.
func (lr *ClientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *ClientLatency) tick(_ types.TickEvent) {
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
