package synchronizer_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/synchronizer"
)

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContext(t *testing.T) {
	eventloop := core.NewEventLoop(10)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.TimeoutEvent{})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextView(t *testing.T) {
	eventloop := core.NewEventLoop(10)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContext(t *testing.T) {
	eventloop := core.NewEventLoop(10)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, nil)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierView(t *testing.T) {
	eventloop := core.NewEventLoop(10)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, &view)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}
