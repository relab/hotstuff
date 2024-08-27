package eventloop_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/relab/hotstuff/eventloop"
)

type testEvent int

func TestHandlerPiped(t *testing.T) {
	const pipeIdThatListens = 0
	el := eventloop.New(10)
	c := make(chan any)
	el.RegisterPipedHandler(pipeIdThatListens, testEvent(0), func(event any) {
		c <- event
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	// wait for the event loop to start
	time.Sleep(1 * time.Millisecond)

	want := testEvent(42)
	el.PipeEvent(pipeIdThatListens, want)

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

func TestHandlerPipedNoResponse(t *testing.T) {
	const pipeIdThatListens = 0
	const pipeIdToEmitTo = 1

	el := eventloop.New(10)
	c := make(chan any)
	el.RegisterPipedHandler(pipeIdThatListens, testEvent(0), func(event any) {
		c <- event
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	// wait for the event loop to start
	time.Sleep(1 * time.Millisecond)

	want := testEvent(42)
	el.PipeEvent(pipeIdToEmitTo, want)

	var event any
	select {
	case <-ctx.Done(): // We expect a timeout since we dont want to listen to this pipe ID
	case event = <-c:
		t.Fatal("received on incorrect pipe")
	}

	_, ok := event.(testEvent)

	if ok {
		t.Fatal("received on incorrect pipe")
	}
}

func TestObserverPiped(t *testing.T) {
	const pipeIdThatListens = 0

	type eventData struct {
		event   any
		handler bool
	}

	el := eventloop.New(10)
	c := make(chan eventData)
	el.RegisterPipedHandler(pipeIdThatListens, testEvent(0), func(event any) {
		c <- eventData{event: event, handler: true}
	})
	el.RegisterPipedObserver(pipeIdThatListens, testEvent(0), func(event any) {
		c <- eventData{event: event, handler: false}
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	want := testEvent(42)
	el.PipeEvent(pipeIdThatListens, want)

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

func TestTickerPiped(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.SkipNow()
		return
	}

	const pipeIdThatListens = 0

	el := eventloop.New(10)
	count := 0
	el.RegisterPipedHandler(pipeIdThatListens, testEvent(0), func(event any) {
		count += int(event.(testEvent))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	rate := 100 * time.Millisecond
	id := el.AddPipedTicker(pipeIdThatListens, rate, func(_ time.Time) (_ any) { return testEvent(1) })

	// sleep a little less than 1 second to ensure we get the expected amount of ticks
	time.Sleep(time.Second - rate/4)
	if expected := int(time.Second / rate); count != expected {
		t.Fatalf("ticker fired %d times in 1 second, expected %d", count, expected)
	}

	// check that the ticker stops correctly
	old := count
	el.RemovePipedTicker(pipeIdThatListens, id)

	// sleep another tick to ensure the ticker has stopped
	time.Sleep(rate)

	if old != count {
		t.Fatal("ticker was not stopped")
	}
}

func TestDelayedEventPiped(t *testing.T) {
	const pipeIdThatListens = 0
	el := eventloop.New(10)
	c := make(chan testEvent)

	el.RegisterPipedHandler(pipeIdThatListens, testEvent(0), func(event any) {
		c <- event.(testEvent)
	})

	// delay the "2" and "3" events until after the first instance of testEvent
	el.DelayUntil(testEvent(0), testEvent(2))
	el.DelayUntil(testEvent(0), testEvent(3))
	// then send the "1" event
	el.PipeEvent(pipeIdThatListens, testEvent(1))

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

func TestHandlerStandard(t *testing.T) {
	el := eventloop.New(10)
	c := make(chan any)
	el.RegisterHandler(testEvent(0), func(event any) {
		c <- event
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go el.Run(ctx)

	// wait for the event loop to start
	time.Sleep(1 * time.Millisecond)

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

func TestObserverStandard(t *testing.T) {
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

func TestTickerStandard(t *testing.T) {
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
	id := el.AddTicker(rate, func(_ time.Time) (_ any) { return testEvent(1) })

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

func TestDelayedEventStandard(t *testing.T) {
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
