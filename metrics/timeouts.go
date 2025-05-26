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
	eventLoop *eventloop.EventLoop,
	metricsLogger Logger,
	id hotstuff.ID,
) {
	vt := &viewTimeouts{
		metricsLogger: metricsLogger,
		id:            id,
	}

	eventLoop.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		vt.viewChange(event.(hotstuff.ViewChangeEvent))
	})

	eventLoop.RegisterHandler(types.TickEvent{}, func(event any) {
		vt.tick(event.(types.TickEvent))
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
