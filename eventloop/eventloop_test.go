package eventloop_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/relab/hotstuff/eventloop"
)

type testEvent int

func TestHandler(t *testing.T) {
	el := eventloop.New(10)
	c := make(chan any)
	el.RegisterHandler(testEvent(0), func(event any) {
		c <- event
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	want := testEvent(42)
	el.AddEvent(want)

	var event any
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

func TestObserver(t *testing.T) {
	type eventData struct {
		event   any
		handler bool
	}

	el := eventloop.New(10)
	c := make(chan eventData)
	el.RegisterHandler(testEvent(0), func(event any) {
		c <- eventData{event: event, handler: true}
	})
	el.RegisterObserver(testEvent(0), func(event any) {
		c <- eventData{event: event, handler: false}
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	want := testEvent(42)
	el.AddEvent(want)

	for i := 0; i < 2; i++ {
		var data eventData
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case data = <-c:
		}

		if i == 0 && data.handler {
			t.Fatalf("expected observer to run first")
		}

		if i == 1 && !data.handler {
			t.Fatalf("expected handler to run second")
		}

		e, ok := data.event.(testEvent)
		if !ok {
			t.Fatalf("wrong type for event: got: %T, want: %T", data, want)
		}

		if e != want {
			t.Fatalf("wrong value for event: got: %v, want: %v", e, want)
		}
	}
}

func TestTicker(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.SkipNow()
		return
	}

	el := eventloop.New(10)
	count := 0
	el.RegisterHandler(testEvent(0), func(event any) {
		count += int(event.(testEvent))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	rate := 100 * time.Millisecond
	id := el.AddTicker(rate, func(tick time.Time) (event any) { return testEvent(1) })

	// sleep a little less than 1 second to ensure we get the expected amount of ticks
	time.Sleep(time.Second - rate/4)
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

func TestDelayedEvent(t *testing.T) {
	el := eventloop.New(10)
	c := make(chan testEvent)

	el.RegisterHandler(testEvent(0), func(event any) {
		c <- event.(testEvent)
	})

	// delay the "2" and "3" events until after the first instance of testEvent
	el.DelayUntil(testEvent(0), testEvent(2))
	el.DelayUntil(testEvent(0), testEvent(3))
	// then send the "1" event
	el.AddEvent(testEvent(1))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	for i := 1; i <= 3; i++ {
		select {
		case event := <-c:
			if testEvent(i) != event {
				t.Errorf("events arrived in the wrong order: want: %d, got: %d", i, event)
			}
		case <-ctx.Done():
			t.Fatalf("timed out")
		}
	}
}

func BenchmarkEventLoopWithObservers(b *testing.B) {
	el := eventloop.New(100)

	for i := 0; i < 100; i++ {
		el.RegisterObserver(testEvent(0), func(event any) {
			if event.(testEvent) != 1 {
				panic("Unexpected value observed")
			}
		})
	}

	for i := 0; i < b.N; i++ {
		el.AddEvent(testEvent(1))
		el.Tick(context.Background())
	}
}

func BenchmarkEventLoopWithUnsafeRunInAddEventHandlers(b *testing.B) {
	el := eventloop.New(100)

	for i := 0; i < 100; i++ {
		el.RegisterHandler(testEvent(0), func(event any) {
			if event.(testEvent) != 1 {
				panic("Unexpected value observed")
			}
		}, eventloop.UnsafeRunInAddEvent())
	}

	for i := 0; i < b.N; i++ {
		el.AddEvent(testEvent(1))
		el.AddEvent(testEvent(1))
		el.AddEvent(testEvent(1))
		el.AddEvent(testEvent(1))
		el.AddEvent(testEvent(1))
		el.Tick(context.Background())
		el.Tick(context.Background())
		el.Tick(context.Background())
		el.Tick(context.Background())
		el.Tick(context.Background())
	}
}

func BenchmarkDelay(b *testing.B) {
	el := eventloop.New(100)

	for i := 0; i < b.N; i++ {
		el.DelayUntil(testEvent(0), testEvent(2))
		el.DelayUntil(testEvent(0), testEvent(3))
		el.AddEvent(testEvent(1))
		el.Tick(context.Background())
		el.Tick(context.Background())
		el.Tick(context.Background())
	}
}
