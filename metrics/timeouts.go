package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/metrics/types"
)

const NameViewTimeouts = "timeouts"

// ViewTimeouts is a metric that measures the number of view timeouts that happen.
type ViewTimeouts struct {
	metricsLogger Logger
	opts          *core.Options

	numViews    uint64
	numTimeouts uint64
}

func NewViewTimeouts(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	metricsLogger Logger,
	opts *core.Options,
) *ViewTimeouts {
	vt := &ViewTimeouts{
		metricsLogger: metricsLogger,
		opts:          opts,
	}
	logger.Info("ViewTimeouts metric enabled.")

	eventLoop.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		vt.viewChange(event.(hotstuff.ViewChangeEvent))
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		vt.tick(event.(types.TickEvent))
	}, eventloop.Prioritize())
	return vt
}

func (vt *ViewTimeouts) viewChange(event hotstuff.ViewChangeEvent) {
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
