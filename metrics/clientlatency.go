package metrics

import (
	"time"

	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterClientMetric("client-latency", func() interface{} {
		return &ClientLatency{}
	})
}

// ClientLatency processes LatencyMeasurementEvents, and writes LatencyMeasurements to the metrics logger.
type ClientLatency struct {
	mods *modules.Modules
	wf   Welford
}

// InitModule gives the module access to the other modules.
func (lr *ClientLatency) InitModule(mods *modules.Modules) {
	lr.mods = mods

	lr.mods.EventLoop().RegisterHandler(client.LatencyMeasurementEvent{}, func(event interface{}) {
		latencyEvent := event.(client.LatencyMeasurementEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	lr.mods.EventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
		lr.tick(event.(types.TickEvent))
	})

	lr.mods.Logger().Info("Client Latency metric enabled")
}

// AddLatency adds a latency data point to the current measurement.
func (lr *ClientLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *ClientLatency) tick(tick types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewClientEvent(uint32(lr.mods.ID()), time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.mods.MetricsLogger().Log(event)
	lr.wf.Reset()
}
