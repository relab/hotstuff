package metrics

import (
	"time"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

// ticker emits TickEvents on the metrics event loop.
type ticker struct {
	tickerID int
	interval time.Duration
	lastTick time.Time
}

// addTicker returns a new ticker.
func addTicker(eventLoop *eventloop.EventLoop, interval time.Duration) {
	t := &ticker{interval: interval}
	t.tickerID = eventLoop.AddTicker(t.interval, t.tick)
}

func (t *ticker) tick(tickTime time.Time) any {
	var event any
	if !t.lastTick.IsZero() {
		event = types.TickEvent{
			LastTick: t.lastTick,
		}
	}
	t.lastTick = tickTime
	return event
}
