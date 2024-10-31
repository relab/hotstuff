package metrics

import (
	"time"

	"github.com/relab/hotstuff/debug"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
)

func init() {
	RegisterReplicaMetric("debug", func() any {
		return &DebugMetrics{
			sequentialPipedCommitHalts: make(map[pipeline.Pipe]int),
			rejectedCommands:           make(map[pipeline.Pipe]int),
		}
	})
}

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type DebugMetrics struct {
	metricsLogger Logger
	opts          *modules.Options
	pipeCount     int

	// metrics
	sequentialPipedCommitHalts map[pipeline.Pipe]int
	rejectedCommands           map[pipeline.Pipe]int
}

// InitModule gives the module access to the other modules.
func (db *DebugMetrics) InitModule(mods *modules.Core, opt modules.InitOptions) {
	var (
		eventLoop *eventloop.EventLoop
		logger    logging.Logger
	)

	mods.Get(
		&db.metricsLogger,
		&db.opts,
		&eventLoop,
		&logger,
	)

	db.pipeCount = opt.PipeCount

	logger.Info("DebugMetrics enabled.")

	eventLoop.RegisterHandler(debug.CommitHaltEvent{}, func(event any) {
		halt := event.(debug.CommitHaltEvent)
		db.sequentialPipedCommitHalts[halt.OnPipe]++
	})

	eventLoop.RegisterHandler(debug.CommandRejectedEvent{}, func(event any) {
		reject := event.(debug.CommandRejectedEvent)
		db.rejectedCommands[reject.OnPipe]++
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		db.tick(event.(types.TickEvent))
	})
}

func (db *DebugMetrics) tick(_ types.TickEvent) {
	var maxPipes pipeline.Pipe = 1
	var startPipe pipeline.Pipe = pipeline.NullPipe

	if db.pipeCount > 0 {
		maxPipes = pipeline.Pipe(db.pipeCount) + 1
		startPipe++
	}

	for pipe := startPipe; pipe < maxPipes; pipe++ {
		db.metricsLogger.Log(&types.DebugMeasurement{
			Event:            types.NewReplicaEvent(uint32(db.opts.ID()), time.Now()),
			OnPipe:           uint32(pipe),
			CommitHalts:      uint32(db.sequentialPipedCommitHalts[pipe]),
			RejectedCommands: uint32(db.rejectedCommands[pipe]),
		})

		db.sequentialPipedCommitHalts[pipe] = 0
		db.rejectedCommands[pipe] = 0
	}

}
