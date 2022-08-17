package synchronizer

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
)

// ViewContext returns a context that is cancelled at the end of a view.
// If view is nil or less than or equal to the current view, the context will be cancelled at the next view change.
func ViewContext(parent context.Context, view *hotstuff.View, eventLoop *eventloop.EventLoop) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterObserver(ViewChangeEvent{}, func(event any) {
		if view == nil || event.(ViewChangeEvent).View >= *view {
			cancel()
		}
	})

	return ctx, func() {
		eventLoop.UnregisterObserver(ViewChangeEvent{}, id)
		cancel()
	}
}
