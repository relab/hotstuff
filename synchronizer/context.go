package synchronizer

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func ViewContext(parent context.Context, eventLoop *core.EventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		if view == nil || event.(ViewChangeEvent).View >= *view {
			cancel()
		}
	}, core.Prioritize(), core.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, id)
		cancel()
	}
}

// TimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
func TimeoutContext(parent context.Context, eventLoop *core.EventLoop) (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := ViewContext(parent, eventLoop, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(_ any) {
		cancel()
	}, core.Prioritize(), core.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, id)
		cancel()
	}
}
