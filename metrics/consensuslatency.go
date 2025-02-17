package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterReplicaMetric("consensus-latency", func() any {
		return &ConsensusLatency{}
	})
}

// ConsensusLatency processes ConsensusLatencyMeasurementEvent, and
// writes ConsensusLatencyMeasurementEvent to the metrics logger.
type ConsensusLatency struct {
	metricsLogger Logger
	opts          *modules.Options

	wf Welford
}

// InitModule gives the module access to the other modules.
func (lr *ConsensusLatency) InitModule(mods *modules.Core) {
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

	eventLoop.RegisterHandler(hotstuff.ConsensusLatencyMeasurementEvent{}, func(event any) {
		latencyEvent := event.(hotstuff.ConsensusLatencyMeasurementEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		lr.tick(event.(types.TickEvent))
	}, eventloop.Prioritize())

	logger.Info("Consensus Latency metric enabled")
}

// AddLatency adds a latency data point to the current measurement.
func (lr *ConsensusLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *ConsensusLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewReplicaEvent(uint32(lr.opts.ID()), time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
