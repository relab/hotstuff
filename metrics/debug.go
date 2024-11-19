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
			commitHalts:      make(map[hotstuff.Instance]int),
			rejectedCommands: make(map[hotstuff.Instance]int),
		}
	})
}

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type DebugMetrics struct {
	metricsLogger Logger
	opts          *modules.Options
	instanceCount int

	// metrics
	commitHalts      map[hotstuff.Instance]int
	rejectedCommands map[hotstuff.Instance]int
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

	db.instanceCount = info.ScopeCount

	logger.Info("DebugMetrics enabled.")

	eventLoop.RegisterHandler(debug.CommitHaltEvent{}, func(event any) {
		halt := event.(debug.CommitHaltEvent)
		db.commitHalts[halt.Instance]++
	})

	eventLoop.RegisterHandler(debug.CommandRejectedEvent{}, func(event any) {
		reject := event.(debug.CommandRejectedEvent)
		db.rejectedCommands[reject.Instance]++
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		db.tick(event.(types.TickEvent))
	})
}

func (db *DebugMetrics) tick(_ types.TickEvent) {
	var maxCi hotstuff.Instance = 1
	var start hotstuff.Instance = hotstuff.ZeroInstance

	if db.instanceCount > 0 {
		maxCi = hotstuff.Instance(db.instanceCount) + 1
		start++
	}

	for instance := start; instance < maxCi; instance++ {
		db.metricsLogger.Log(&types.DebugMeasurement{
			Event:            types.NewReplicaEvent(uint32(db.opts.ID()), time.Now()),
			Instance:         uint32(instance),
			CommitHalts:      uint32(db.commitHalts[instance]),
			RejectedCommands: uint32(db.rejectedCommands[instance]),
		})

		db.commitHalts[instance] = 0
		db.rejectedCommands[instance] = 0
	}

}
