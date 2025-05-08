package viewduration

import (
	"time"

	"github.com/relab/hotstuff/modules"
)

type fixedViewDuration struct {
	duration time.Duration
}

// NewFixedViewDuration returns a ViewDuration with a fixed duration.
func NewFixed(duration time.Duration) modules.ViewDuration {
	return &fixedViewDuration{
		duration: duration,
	}
}

// Duration returns the fixed duration.
func (f *fixedViewDuration) Duration() time.Duration {
	return f.duration
}

// ViewStarted does nothing for FixedViewDuration.
func (f *fixedViewDuration) ViewStarted() {}

// ViewSucceeded does nothing for FixedViewDuration.
func (f *fixedViewDuration) ViewSucceeded() {}

// ViewTimeout does nothing for FixedViewDuration.
func (f *fixedViewDuration) ViewTimeout() {}
