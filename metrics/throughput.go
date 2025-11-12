package metrics

import (
	"time"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/metrics/types"
	"google.golang.org/protobuf/types/known/durationpb"
)

const NameThroughput = "throughput"

// throughput measures throughput in commits per second, and commands per second.
type throughput struct {
	metricsLogger Logger
	id            hotstuff.ID

	commitCount  uint64
	commandCount uint64
}

func enableThroughput(
	el *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	t := &throughput{
		metricsLogger: metricsLogger,
		id:            id,
	}
	eventloop.Register(el, func(commitEvent clientpb.ExecuteEvent) {
		t.recordCommit(len(commitEvent.Batch.Commands))
	})
	eventloop.Register(el, func(tickEvent types.TickEvent) {
		t.tick(tickEvent)
	}, eventloop.Prioritize())
}

func (t *throughput) recordCommit(commands int) {
	t.commitCount++
	t.commandCount += uint64(commands)
}

func (t *throughput) tick(tick types.TickEvent) {
	now := time.Now()
	event := &types.ThroughputMeasurement{
		Event:    types.NewReplicaEvent(uint32(t.id), now),
		Commits:  t.commitCount,
		Commands: t.commandCount,
		Duration: durationpb.New(now.Sub(tick.LastTick)),
	}
	t.metricsLogger.Log(event)
	// reset count for next tick
	t.commandCount = 0
	t.commitCount = 0
}
