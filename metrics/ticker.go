package metrics

import (
	"time"

	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/metrics/types"
)

// Ticker emits TickEvents on the metrics event loop.
type Ticker struct {
	tickerID int
	interval time.Duration
	lastTick time.Time
}

// NewTicker returns a new ticker.
func NewTicker(eventLoop *eventloop.EventLoop, interval time.Duration) *Ticker {
	t := &Ticker{interval: interval}
	t.tickerID = eventLoop.AddTicker(t.interval, t.tick)
	return t
}

func (t *Ticker) tick(tickTime time.Time) any {
	var event any
	if !t.lastTick.IsZero() {
		event = types.TickEvent{
			LastTick: t.lastTick,
		}
	}
	t.lastTick = tickTime
	return event
}
