package eventloop_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContext(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	ctx, cancel := eventloop.TimeoutContext()
	defer cancel()

	eventloop.AddEvent(hotstuff.TimeoutEvent{})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestTimeoutContextView tests that a timeout context is canceled after receiving a view change event.
func TestTimeoutContextView(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	ctx, cancel := eventloop.TimeoutContext()
	defer cancel()

	eventloop.AddEvent(hotstuff.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContext(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	ctx, cancel := eventloop.ViewContext(nil)
	defer cancel()

	eventloop.AddEvent(hotstuff.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}

// TestViewContextEarlierView tests that a view context is not canceled when receiving a view change event for an earlier view.
func TestViewContextEarlierView(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	v := hotstuff.View(1)
	ctx, cancel := eventloop.ViewContext(&v)
	defer cancel()

	eventloop.AddEvent(hotstuff.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}
