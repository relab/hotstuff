package metrics

import (
	"time"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

// Ticker emits TickEvents on the metrics event loop.
type Ticker struct {
	tickerID int
	interval time.Duration
	lastTick time.Time
}

// NewTicker returns a new ticker.
func NewTicker(interval time.Duration) *Ticker {
	return &Ticker{interval: interval}
}

// InitModule gives the module access to the other modules.
func (t *Ticker) InitModule(mods *modules.Core) {
	var eventLoop *eventloop.EventLoop

	mods.Get(&eventLoop)

	t.tickerID = eventLoop.AddTicker(t.interval, t.tick)
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
