package metrics

import (
	"time"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	RegisterReplicaMetric("throughput", func() any {
		return &Throughput{
			commitCount:  make(map[pipeline.Pipe]uint64),
			commandCount: make(map[pipeline.Pipe]uint64),
		}
	})
}

// Throughput measures throughput in commits per second, and commands per second.
type Throughput struct {
	metricsLogger Logger
	opts          *modules.Options
	pipeCount     int

	commitCount  map[pipeline.Pipe]uint64
	commandCount map[pipeline.Pipe]uint64
}

// InitModule gives the module access to the other modules.
func (t *Throughput) InitModule(mods *modules.Core, opt modules.InitOptions) {
	var (
		eventLoop *eventloop.EventLoop
		logger    logging.Logger
	)
	mods.Get(
		&t.metricsLogger,
		&t.opts,
		&eventLoop,
		&logger,
	)

	t.pipeCount = opt.PipeCount

	eventLoop.RegisterHandler(hotstuff.CommitEvent{}, func(event any) {
		commitEvent := event.(hotstuff.CommitEvent)
		t.recordCommit(commitEvent.OnPipe, commitEvent.Commands)
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		t.tick(event.(types.TickEvent))
	})

	logger.Info("Throughput metric enabled")
}

func (t *Throughput) recordCommit(onPipe pipeline.Pipe, commands int) {
	t.commitCount[onPipe]++
	t.commandCount[onPipe] += uint64(commands)
}

func (t *Throughput) tick(tick types.TickEvent) {
	now := time.Now()

	var totalCommands uint64 = 0
	var totalCommits uint64 = 0
	var maxPipes pipeline.Pipe = 1
	var startPipe pipeline.Pipe = pipeline.NullPipe

	if t.pipeCount > 0 {
		maxPipes = pipeline.Pipe(t.pipeCount) + 1
		startPipe++
	}

	for pipe := startPipe; pipe < maxPipes; pipe++ {
		event := &types.ThroughputMeasurement{
			Event:    types.NewReplicaEvent(uint32(t.opts.ID()), now),
			Commits:  t.commitCount[pipe],
			Commands: t.commandCount[pipe],
			Duration: durationpb.New(now.Sub(tick.LastTick)),
			Pipe:     uint32(pipe),
		}
		t.metricsLogger.Log(event)
		totalCommands += t.commandCount[pipe]
		totalCommits += t.commitCount[pipe]
		// reset count for next tick
		t.commandCount[pipe] = 0
		t.commitCount[pipe] = 0
	}

	if t.pipeCount > 0 {
		event := &types.TotalThroughputMeasurement{
			Event:     types.NewReplicaEvent(uint32(t.opts.ID()), now),
			Commits:   totalCommits,
			Commands:  totalCommands,
			Duration:  durationpb.New(now.Sub(tick.LastTick)),
			PipeCount: uint32(t.pipeCount),
		}
		t.metricsLogger.Log(event)
	}
}
