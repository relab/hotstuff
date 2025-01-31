package view

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
)

// This file provides several functions for creating contexts with lifespans that are tied to synchronizer events.

// Context returns a context that is canceled at the end of view.
// If view is nil or less than or equal to the current view, the context will be canceled at the next view change.
func Context(parent context.Context, eventLoop *eventloop.EventLoop, view *hotstuff.View) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	id := eventLoop.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		if view == nil || event.(hotstuff.ViewChangeEvent).View >= *view {
			cancel()
		}
	}, eventloop.Prioritize(), eventloop.UnsafeRunInAddEvent())

	return ctx, func() {
		eventLoop.UnregisterHandler(hotstuff.ViewChangeEvent{}, id)
		cancel()
	}
}
