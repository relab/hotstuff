package synchronizer

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/pipeline"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// ViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func ViewContext(parent context.Context, eventLoop *eventloop.EventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		if view == nil || event.(ViewChangeEvent).View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, pipeline.NullPipe, id)
		cancel()
	}
}

// TimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
func TimeoutContext(parent context.Context, eventLoop *eventloop.EventLoop) (context.Context, context.CancelFunc) {
	// ViewContext handles view-change case.
	ctx, cancel := ViewContext(parent, eventLoop, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(_ any) {
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, pipeline.NullPipe, id)
		cancel()
	}
}

// PipedViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
// Pipe is null-pipe, returns regular PipedViewContext.
func PipedViewContext(parent context.Context, eventLoop *eventloop.EventLoop, pipe pipeline.Pipe, view *hotstuff.View) (context.Context, context.CancelFunc) {
	if pipe == pipeline.NullPipe {
		return ViewContext(parent, eventLoop, view)
	}

	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		myPipe := pipe
		viewChangeEvent := event.(ViewChangeEvent)
		if viewChangeEvent.Pipe != myPipe {
			panic("something is wrong")
		}
		if view == nil || viewChangeEvent.View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToPipe(pipe))

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, pipe, id)
		cancel()
	}
}

// PipedTimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
// Pipe is null-pipe, returns regular TimeoutContext.
func PipedTimeoutContext(parent context.Context, eventLoop *eventloop.EventLoop, pipe pipeline.Pipe) (context.Context, context.CancelFunc) {
	if pipe == pipeline.NullPipe {
		return TimeoutContext(parent, eventLoop)
	}

	// ViewContext handles view-change case.
	ctx, cancel := PipedViewContext(parent, eventLoop, pipe, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		myPipe := pipe
		timeoutEvent := event.(TimeoutEvent)
		if timeoutEvent.Pipe != myPipe {
			panic("something is wrong")
		}
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToPipe(pipe))

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, pipe, id)
		cancel()
	}
}
