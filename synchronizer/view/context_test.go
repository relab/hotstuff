package view_test

import (
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/synchronizer/view"
)

// TestViewContext tests that a view context is canceled after receiving a view change event.
func TestViewContext(t *testing.T) {
	logger := logging.New("test")
	eventloop := eventloop.New(logger, 10)
	ctx, cancel := view.Context(context.Background(), eventloop, nil)
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
	ctx, cancel := view.Context(context.Background(), eventloop, &v)
	defer cancel()

	eventloop.AddEvent(hotstuff.ViewChangeEvent{View: 0})

	if ctx.Err() != nil {
		t.Error("Context canceled")
	}
}
