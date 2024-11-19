package metrics

import (
	"time"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	RegisterReplicaMetric("throughput", func() any {
		return &Throughput{
			commitCount:  make(map[hotstuff.Instance]uint64),
			commandCount: make(map[hotstuff.Instance]uint64),
		}
	})
}

// Throughput measures throughput in commits per second, and commands per second.
type Throughput struct {
	metricsLogger Logger
	opts          *modules.Options
	instanceCount int

	commitCount  map[hotstuff.Instance]uint64
	commandCount map[hotstuff.Instance]uint64
}

// InitModule gives the module access to the other modules.
func (t *Throughput) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	var (
		eventLoop *eventloop.ScopedEventLoop
		logger    logging.Logger
	)
	mods.Get(
		&t.metricsLogger,
		&t.opts,
		&eventLoop,
		&logger,
	)

	t.instanceCount = info.ScopeCount

	eventLoop.RegisterHandler(hotstuff.CommitEvent{}, func(event any) {
		commitEvent := event.(hotstuff.CommitEvent)
		t.recordCommit(commitEvent.Instance, commitEvent.Commands)
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		t.tick(event.(types.TickEvent))
	})

	logger.Info("Throughput metric enabled")
}

func (t *Throughput) recordCommit(instance hotstuff.Instance, commands int) {
	t.commitCount[instance]++
	t.commandCount[instance] += uint64(commands)
}

func (t *Throughput) tick(tick types.TickEvent) {
	now := time.Now()

	var totalCommands uint64 = 0
	var totalCommits uint64 = 0
	var maxCi hotstuff.Instance = 1
	var start hotstuff.Instance = hotstuff.ZeroInstance

	if t.instanceCount > 0 {
		maxCi = hotstuff.Instance(t.instanceCount) + 1
		start++
	}

	for instance := start; instance < maxCi; instance++ {
		event := &types.ThroughputMeasurement{
			Event:    types.NewReplicaEvent(uint32(t.opts.ID()), now),
			Commits:  t.commitCount[instance],
			Commands: t.commandCount[instance],
			Duration: durationpb.New(now.Sub(tick.LastTick)),
			Instance: uint32(instance),
		}
		t.metricsLogger.Log(event)
		totalCommands += t.commandCount[instance]
		totalCommits += t.commitCount[instance]
		// reset count for next tick
		t.commandCount[instance] = 0
		t.commitCount[instance] = 0
	}

	if t.instanceCount > 0 {
		event := &types.TotalThroughputMeasurement{
			Event:         types.NewReplicaEvent(uint32(t.opts.ID()), now),
			Commits:       totalCommits,
			Commands:      totalCommands,
			Duration:      durationpb.New(now.Sub(tick.LastTick)),
			InstanceCount: uint32(t.instanceCount),
		}
		t.metricsLogger.Log(event)
	}
}
