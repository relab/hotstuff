package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

func init() {
	RegisterReplicaMetric("timeouts", func() any {
		return &ViewTimeouts{}
	})
}

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type ViewTimeouts struct {
	metricsLogger Logger
	opts          *modules.Options

	numViews    uint64
	numTimeouts uint64
}

// InitModule gives the module access to the other modules.
func (vt *ViewTimeouts) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	var (
		eventLoop *eventloop.ScopedEventLoop
		logger    logging.Logger
	)

	mods.Get(
		&vt.metricsLogger,
		&vt.opts,
		&eventLoop,
		&logger,
	)

	logger.Info("ViewTimeouts metric enabled.")

	if info.IsPipeliningEnabled {
		for pipe := hotstuff.Pipe(1); pipe <= hotstuff.Pipe(info.ScopeCount); pipe++ {
			eventLoop.RegisterHandler(synchronizer.ViewChangeEvent{}, func(event any) {
				vt.viewChange(event.(synchronizer.ViewChangeEvent))
			}, eventloop.RespondToScope(pipe))
		}
	} else {

		eventLoop.RegisterHandler(synchronizer.ViewChangeEvent{}, func(event any) {
			vt.viewChange(event.(synchronizer.ViewChangeEvent))
		})
	}

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		vt.tick(event.(types.TickEvent))
	})
}

func (vt *ViewTimeouts) viewChange(event synchronizer.ViewChangeEvent) {
	vt.numViews++
	if event.Timeout {
		vt.numTimeouts++
	}
}

func (vt *ViewTimeouts) tick(_ types.TickEvent) {
	vt.metricsLogger.Log(&types.ViewTimeouts{
		Event:    types.NewReplicaEvent(uint32(vt.opts.ID()), time.Now()),
		Views:    vt.numViews,
		Timeouts: vt.numTimeouts,
	})
	vt.numViews = 0
	vt.numTimeouts = 0
}
