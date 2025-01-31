package metrics

import (
	"time"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/metrics/types"
	"google.golang.org/protobuf/types/known/durationpb"
)

const NameThroughput = "throughput"

// Throughput measures throughput in commits per second, and commands per second.
type Throughput struct {
	metricsLogger Logger
	opts          *core.Options

	commitCount  uint64
	commandCount uint64
}

func NewThroughput(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	opts *core.Options,
) *Throughput {
	t := &Throughput{
		metricsLogger: metricsLogger,
		opts:          opts,
	}
	eventLoop.RegisterHandler(hotstuff.CommitEvent{}, func(event any) {
		commitEvent := event.(hotstuff.CommitEvent)
		t.recordCommit(commitEvent.Commands)
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		t.tick(event.(types.TickEvent))
	}, eventloop.Prioritize())

	logger.Info("Throughput metric enabled")
	return t
}

func (t *Throughput) recordCommit(commands int) {
	t.commitCount++
	t.commandCount += uint64(commands)
}

func (t *Throughput) tick(tick types.TickEvent) {
	now := time.Now()
	event := &types.ThroughputMeasurement{
		Event:    types.NewReplicaEvent(uint32(t.opts.ID()), now),
		Commits:  t.commitCount,
		Commands: t.commandCount,
		Duration: durationpb.New(now.Sub(tick.LastTick)),
	}
	t.metricsLogger.Log(event)
	// reset count for next tick
	t.commandCount = 0
	t.commitCount = 0
}
