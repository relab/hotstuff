package timeout

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/protocol/synchronizer/view"
)

// Context returns a context that is canceled either when a timeout occurs, or when the view changes.
func Context(parent context.Context, eventLoop *eventloop.EventLoop) (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := view.Context(parent, eventLoop, nil)

	id := eventLoop.RegisterHandler(hotstuff.TimeoutEvent{}, func(_ any) {
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(hotstuff.TimeoutEvent{}, id)
		cancel()
	}
}
