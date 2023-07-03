package synchronizer_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/synchronizer"
)

// TestTimeoutContext tests that a timeout context is cancelled after receiving a timeout event.
func TestTimeoutContext(t *testing.T) {
	eventloop := eventloop.New(10)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.TimeoutEvent{})

	if ctx.Err() != context.Canceled {
		t.Error("Context not cancelled")
	}
}

// TestTimeoutContextView tests that a timeout context is cancelled after receiving a view change event.
func TestTimeoutContextView(t *testing.T) {
	eventloop := eventloop.New(10)
	ctx, cancel := synchronizer.TimeoutContext(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not cancelled")
	}
}

// TestViewContext tests that a view context is cancelled after receiving a view change event.
func TestViewContext(t *testing.T) {
	eventloop := eventloop.New(10)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, nil)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not cancelled")
	}
}

// TestViewContextEarlierView tests that a view context is not cancelled when receiving a view change event for an earlier view.
func TestViewContextEarlierView(t *testing.T) {
	eventloop := eventloop.New(10)
	view := hotstuff.View(1)
	ctx, cancel := synchronizer.ViewContext(context.Background(), eventloop, &view)
	defer cancel()

	eventloop.AddEvent(synchronizer.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context cancelled")
	}
}
