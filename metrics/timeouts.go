package metrics

import (
	"time"

	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

func init() {
	RegisterReplicaMetric("timeouts", func() interface{} {
		return &ViewTimeouts{}
	})
}

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type ViewTimeouts struct {
	mods        *modules.Modules
	numViews    uint64
	numTimeouts uint64
}

// InitModule gives the module access to the other modules.
func (vt *ViewTimeouts) InitModule(mods *modules.Modules) {
	vt.mods = mods

	vt.mods.Logger().Info("ViewTimeouts metric enabled.")

	vt.mods.EventLoop().RegisterHandler(synchronizer.ViewChangeEvent{}, func(event interface{}) {
		vt.viewChange(event.(synchronizer.ViewChangeEvent))
	})

	vt.mods.EventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
		vt.tick(event.(types.TickEvent))
	})
}

func (vt *ViewTimeouts) viewChange(event synchronizer.ViewChangeEvent) {
	vt.numViews++
	if event.Timeout {
		vt.numTimeouts++
	}
}

func (vt *ViewTimeouts) tick(event types.TickEvent) {
	vt.mods.MetricsLogger().Log(&types.ViewTimeouts{
		Event:    types.NewReplicaEvent(uint32(vt.mods.ID()), time.Now()),
		Views:    vt.numViews,
		Timeouts: vt.numTimeouts,
	})
	vt.numViews = 0
	vt.numTimeouts = 0
}
