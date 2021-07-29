package eventloop_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/relab/hotstuff/eventloop"
)

type testEvent int

func TestSyncHandler(t *testing.T) {
	el := eventloop.New(10)
	c := make(chan interface{})
	el.RegisterHandler(testEvent(0), func(event interface{}) {
		c <- event
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	want := testEvent(42)
	el.AddEvent(want)

	var event interface{}
	select {
	case <-ctx.Done():
		t.Fatal("timed out")
	case event = <-c:
	}

	e, ok := event.(testEvent)
	if !ok {
		t.Fatalf("wrong type for event: got: %T, want: %T", event, want)
	}

	if e != want {
		t.Fatalf("wrong value for event: got: %v, want: %v", e, want)
	}
}

func TestTicker(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.SkipNow()
		return
	}

	el := eventloop.New(10)
	count := 0
	el.RegisterHandler(testEvent(0), func(event interface{}) {
		count += int(event.(testEvent))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	rate := 100 * time.Millisecond
	id := el.AddTicker(rate, func(tick time.Time) (event interface{}) { return testEvent(1) })

	// sleep a little longer than 1 second to ensure we get the expected amount of ticks
	time.Sleep(time.Second + rate/4)
	if expected := int(time.Second / rate); count != expected {
		t.Fatalf("ticker fired %d times in 1 second, expected %d", count, expected)
	}

	// check that the ticker stops correctly
	old := count
	el.RemoveTicker(id)

	// sleep another tick to ensure the ticker has stopped
	time.Sleep(rate)

	if old != count {
		t.Fatal("ticker was not stopped")
	}
}
