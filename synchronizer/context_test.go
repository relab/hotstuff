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
	eventloop := eventloop.NewPiped(10, 0)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.TimeoutEvent{})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextView(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 0)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContext(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 0)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, nil)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierView(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 0)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, &view)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContextPiped(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 1)
	ctx, cancel := synchronizer.PipedTimeoutContext(context.Background(), eventloop, hotstuff.Instance(1))
	defer cancel()

	eventloop.PipeEvent(hotstuff.Instance(1), synchronizer.TimeoutEvent{Instance: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextViewPiped(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 1)
	ctx, cancel := synchronizer.PipedTimeoutContext(context.Background(), eventloop, hotstuff.Instance(1))
	defer cancel()

	eventloop.PipeEvent(hotstuff.Instance(1), synchronizer.ViewChangeEvent{View: 1, Instance: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContextPiped(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 1)
	ctx, cancel := synchronizer.PipedViewContext(context.Background(), eventloop, hotstuff.Instance(1), nil)
	defer cancel()

	eventloop.PipeEvent(hotstuff.Instance(1), synchronizer.ViewChangeEvent{View: 1, Instance: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierViewPiped(t *testing.T) {
	eventloop := eventloop.NewPiped(10, 1)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.PipedViewContext(context.Background(), eventloop, hotstuff.Instance(1), &view)
	defer cancel()

	eventloop.PipeEvent(hotstuff.Instance(1), synchronizer.ViewChangeEvent{View: 0, Instance: 1})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}
