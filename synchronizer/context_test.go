package synchronizer_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/synchronizer"
)

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContext(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 0)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.TimeoutEvent{})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextView(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 0)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContext(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 0)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, nil)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierView(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 0)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, &view)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContextScoped(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 1)
	ctx, cancel := synchronizer.ScopedTimeoutContext(context.Background(), eventloop, hotstuff.Pipe(1))
	defer cancel()

	eventloop.AddScopedEvent(hotstuff.Pipe(1), synchronizer.TimeoutEvent{Pipe: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextViewScoped(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 1)
	ctx, cancel := synchronizer.ScopedTimeoutContext(context.Background(), eventloop, hotstuff.Pipe(1))
	defer cancel()

	eventloop.AddScopedEvent(hotstuff.Pipe(1), synchronizer.ViewChangeEvent{View: 1, Pipe: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContextScoped(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 1)
	ctx, cancel := synchronizer.ScopedViewContext(context.Background(), eventloop, hotstuff.Pipe(1), nil)
	defer cancel()

	eventloop.AddScopedEvent(hotstuff.Pipe(1), synchronizer.ViewChangeEvent{View: 1, Pipe: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierViewScoped(t *testing.T) {
	eventloop := eventloop.NewScoped(10, 1)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.ScopedViewContext(context.Background(), eventloop, hotstuff.Pipe(1), &view)
	defer cancel()

	eventloop.AddScopedEvent(hotstuff.Pipe(1), synchronizer.ViewChangeEvent{View: 0, Pipe: 1})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}
