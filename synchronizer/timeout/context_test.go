package timeout_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/synchronizer/timeout"
)

// TestTimeoutContext tests that a timeout context is canceled after receiving a timeout event.
func TestTimeoutContext(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	ctx, cancel := timeout.Context(context.Background(), eventloop)
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
	ctx, cancel := timeout.Context(context.Background(), eventloop)
	defer cancel()

	eventloop.AddEvent(hotstuff.ViewChangeEvent{View: 1})

	if ctx.Err() != context.Canceled {
		t.Error("Context not canceled")
	}
}
