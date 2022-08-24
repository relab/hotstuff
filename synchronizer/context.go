package synchronizer

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is cancelled at the end of a view.
// If view is nil or less than or equal to the current view, the context will be cancelled at the next view change.
//
// ViewContext should probably not be used for operations running on the event loop, because
func ViewContext(parent context.Context, eventLoop *eventloop.EventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.
		WithOptions(eventloop.WithPriority(), eventloop.RunAsync()).
		RegisterHandler(ViewChangeEvent{}, func(event any) {
			if view == nil || event.(ViewChangeEvent).View >= *view {
				cancel()
			}
		})

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, id)
		cancel()
	}
}

// TimeoutContext returns a context that is cancelled either when a timeout occurs, or when the view changes.
func TimeoutContext(parent context.Context, eventLoop *eventloop.EventLoop) (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := ViewContext(parent, eventLoop, nil)

	id := eventLoop.
		WithOptions(eventloop.RunAsync(), eventloop.WithPriority()).
		RegisterHandler(TimeoutEvent{}, func(event any) {
			cancel()
		})

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, id)
		cancel()
	}
}
