package metrics

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

const NameViewTimeouts = "timeouts"

// viewTimeouts is a metric that measures the number of view timeouts that happen.
type viewTimeouts struct {
	metricsLogger Logger
	id            hotstuff.ID

	numViews    uint64
	numTimeouts uint64
}

func enableViewTimeouts(
	el *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	vt := &viewTimeouts{
		metricsLogger: metricsLogger,
		id:            id,
	}
	eventloop.Register(el, func(event hotstuff.ViewChangeEvent) {
		vt.viewChange(event)
	})
	eventloop.Register(el, func(tickEvent types.TickEvent) {
		vt.tick(tickEvent)
	}, eventloop.Prioritize())
}

func (vt *viewTimeouts) viewChange(event hotstuff.ViewChangeEvent) {
	vt.numViews++
	if event.Timeout {
		vt.numTimeouts++
	}
}

func (vt *viewTimeouts) tick(_ types.TickEvent) {
	vt.metricsLogger.Log(&types.ViewTimeouts{
		Event:    types.NewReplicaEvent(uint32(vt.id), time.Now()),
		Views:    vt.numViews,
		Timeouts: vt.numTimeouts,
	})
	vt.numViews = 0
	vt.numTimeouts = 0
}
