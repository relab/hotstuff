package eventloop

import (
	"context"

	"github.com/relab/hotstuff"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func (el *EventLoop) ViewContext(view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(el.Context())

	id := el.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		if view == nil || event.(hotstuff.ViewChangeEvent).View >= *view {
			cancel()
		}
	}, Prioritize(), UnsafeRunInAddEvent())

	return ctx, func() {
		el.UnregisterHandler(hotstuff.ViewChangeEvent{}, id)
		cancel()
	}
}

// TimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
func (el *EventLoop) TimeoutContext() (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := el.ViewContext(nil)

	id := el.RegisterHandler(hotstuff.TimeoutEvent{}, func(_ any) {
		cancel()
	}, Prioritize(), UnsafeRunInAddEvent())

	return ctx, func() {
		el.UnregisterHandler(hotstuff.TimeoutEvent{}, id)
		cancel()
	}
}
