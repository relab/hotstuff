package synchronizer

import (
	"context"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func ViewContext(parent context.Context, eventLoop *eventloop.ScopedEventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		if view == nil || event.(ViewChangeEvent).View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, hotstuff.NullPipe, id)
		cancel()
	}
}

// TimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
func TimeoutContext(parent context.Context, eventLoop *eventloop.ScopedEventLoop) (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := ViewContext(parent, eventLoop, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(_ any) {
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, hotstuff.NullPipe, id)
		cancel()
	}
}

// ScopedViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
// If pipe is null pipe, returns regular ScopedViewContext.
func ScopedViewContext(parent context.Context, eventLoop *eventloop.ScopedEventLoop, pipe hotstuff.Pipe, view *hotstuff.View) (context.Context, context.CancelFunc) {
	if pipe == hotstuff.NullPipe {
		return ViewContext(parent, eventLoop, view)
	}

	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		myScope := pipe
		viewChangeEvent := event.(ViewChangeEvent)
		if viewChangeEvent.Pipe != myScope {
			panic(fmt.Sprintf("incorrect pipe: want=%d, got=%d", myScope, viewChangeEvent.Pipe))
		}
		if view == nil || viewChangeEvent.View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToScope(pipe))

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, pipe, id)
		cancel()
	}
}

// ScopedTimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
// If pipe is NullPipe, returns regular TimeoutContext.
func ScopedTimeoutContext(parent context.Context, eventLoop *eventloop.ScopedEventLoop, pipe hotstuff.Pipe) (context.Context, context.CancelFunc) {
	if pipe == hotstuff.NullPipe {
		return TimeoutContext(parent, eventLoop)
	}

	// ViewContext handles view-change case.
	ctx, cancel := ScopedViewContext(parent, eventLoop, pipe, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		myScope := pipe
		timeoutEvent := event.(TimeoutEvent)
		if timeoutEvent.Pipe != myScope {
			panic(fmt.Sprintf("incorrect pipe: want=%d, got=%d", myScope, timeoutEvent.Pipe))
		}
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToScope(pipe))

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, pipe, id)
		cancel()
	}
}
