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
func ViewContext(parent context.Context, eventLoop *eventloop.EventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		if view == nil || event.(ViewChangeEvent).View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, hotstuff.ZeroInstance, id)
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
		eventLoop.UnregisterHandler(TimeoutEvent{}, hotstuff.ZeroInstance, id)
		cancel()
	}
}

// PipedViewContext returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
// If instance is ZeroInstance, returns regular PipedViewContext.
func PipedViewContext(parent context.Context, eventLoop *eventloop.EventLoop, instance hotstuff.Instance, view *hotstuff.View) (context.Context, context.CancelFunc) {
	if instance == hotstuff.ZeroInstance {
		return ViewContext(parent, eventLoop, view)
	}

	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(ViewChangeEvent{}, func(event any) {
		myPipe := instance
		viewChangeEvent := event.(ViewChangeEvent)
		if viewChangeEvent.Instance != myPipe {
			panic(fmt.Sprintf("incorrect consensus instance: want=%d, got=%d", myPipe, viewChangeEvent.Instance))
		}
		if view == nil || viewChangeEvent.View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToInstance(instance))

	return ctx, func() {
		eventLoop.UnregisterHandler(ViewChangeEvent{}, instance, id)
		cancel()
	}
}

// PipedTimeoutContext returns a context that is canceled either when a timeout occurs, or when the view changes.
// If instance is ZeroInstance, returns regular TimeoutContext.
func PipedTimeoutContext(parent context.Context, eventLoop *eventloop.EventLoop, instance hotstuff.Instance) (context.Context, context.CancelFunc) {
	if instance == hotstuff.ZeroInstance {
		return TimeoutContext(parent, eventLoop)
	}

	// ViewContext handles view-change case.
	ctx, cancel := PipedViewContext(parent, eventLoop, instance, nil)

	id := eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		myPipe := instance
		timeoutEvent := event.(TimeoutEvent)
		if timeoutEvent.Instance != myPipe {
			panic(fmt.Sprintf("incorrect consensus instance: want=%d, got=%d", myPipe, timeoutEvent.Instance))
		}
		cancel()
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent(), eventloop.RespondToInstance(instance))

	return ctx, func() {
		eventLoop.UnregisterHandler(TimeoutEvent{}, instance, id)
		cancel()
	}
}
