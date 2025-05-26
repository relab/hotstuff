package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

const NameConsensusLatency = "consensus-latency"

// ConsensusLatency processes consensus latency measurements and writes them to the metrics logger.
type ConsensusLatency struct {
	metricsLogger Logger
	id            hotstuff.ID
	wf            Welford
}

// InitModule gives the module access to the other modules.
func enableConsensusLatency(
	eventLoop *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	lr := ConsensusLatency{
		metricsLogger: metricsLogger,
		id:            id,
	}

	eventLoop.RegisterHandler(hotstuff.ConsensusLatencyEvent{}, func(event any) {
		latencyEvent := event.(hotstuff.ConsensusLatencyEvent)
		lr.addLatency(latencyEvent.Latency)
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		lr.tick(event.(types.TickEvent))
	}, eventloop.Prioritize())

}

// AddLatency adds a latency data point to the current measurement.
func (lr *ConsensusLatency) addLatency(latency time.Duration) {
	millis := float64(latency) / float64(time.Millisecond)
	lr.wf.Update(millis)
}

func (lr *ConsensusLatency) tick(_ types.TickEvent) {
	mean, variance, count := lr.wf.Get()
	event := &types.LatencyMeasurement{
		Event:    types.NewReplicaEvent(uint32(lr.id), time.Now()),
		Latency:  mean,
		Variance: variance,
		Count:    count,
	}
	lr.metricsLogger.Log(event)
	lr.wf.Reset()
}
