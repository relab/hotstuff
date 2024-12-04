package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/debug"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterReplicaMetric("debug", func() any {
		return &DebugMetrics{
			commitHalts:      make(map[hotstuff.Pipe]int),
			rejectedCommands: make(map[hotstuff.Pipe]int),
		}
	})
}

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type DebugMetrics struct {
	metricsLogger Logger
	opts          *modules.Options
	pipeCount     int

	// metrics
	commitHalts      map[hotstuff.Pipe]int
	rejectedCommands map[hotstuff.Pipe]int
}

// InitModule gives the module access to the other modules.
func (db *DebugMetrics) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	var (
		eventLoop *eventloop.ScopedEventLoop
		logger    logging.Logger
	)

	mods.Get(
		&db.metricsLogger,
		&db.opts,
		&eventLoop,
		&logger,
	)

	db.pipeCount = info.ScopeCount

	logger.Info("DebugMetrics enabled.")

	eventLoop.RegisterHandler(debug.CommitHaltEvent{}, func(event any) {
		halt := event.(debug.CommitHaltEvent)
		db.commitHalts[halt.Pipe]++
	})

	eventLoop.RegisterHandler(debug.CommandRejectedEvent{}, func(event any) {
		reject := event.(debug.CommandRejectedEvent)
		db.rejectedCommands[reject.Pipe]++
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		db.tick(event.(types.TickEvent))
	})
}

func (db *DebugMetrics) tick(_ types.TickEvent) {
	var maxCi hotstuff.Pipe = 1
	var start hotstuff.Pipe = hotstuff.NullPipe

	if db.pipeCount > 0 {
		maxCi = hotstuff.Pipe(db.pipeCount) + 1
		start++
	}

	for pipe := start; pipe < maxCi; pipe++ {
		db.metricsLogger.Log(&types.DebugMeasurement{
			Event:            types.NewReplicaEvent(uint32(db.opts.ID()), time.Now()),
			Pipe:             uint32(pipe),
			CommitHalts:      uint32(db.commitHalts[pipe]),
			RejectedCommands: uint32(db.rejectedCommands[pipe]),
		})

		db.commitHalts[pipe] = 0
		db.rejectedCommands[pipe] = 0
	}

}
