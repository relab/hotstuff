package eventloop

import (
	"context"

	"github.com/relab/hotstuff"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func (el *EventLoop) ViewContext(view *hotstuff.View) (context.Context, context.CancelFunc) {
	parentCtx := el.Context()
	if parentCtx == nil {
		// Fallback to Background context to prevent panic from context.WithCancel(nil)
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)

	unregister := Register(el, func(event hotstuff.ViewChangeEvent) {
		if view == nil || event.View >= *view {
			cancel()
		}
	}, Prioritize(), UnsafeRunInAddEvent())

	return ctx, func() {
		unregister()
		cancel()
	}
}

// TimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
func (el *EventLoop) TimeoutContext() (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := el.ViewContext(nil)

	unregister := Register(el, func(_ hotstuff.TimeoutEvent) {
		cancel()
	}, Prioritize(), UnsafeRunInAddEvent())

	return ctx, func() {
		unregister()
		cancel()
	}
}
