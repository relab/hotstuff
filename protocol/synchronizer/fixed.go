package synchronizer

import (
	"time"
)

type FixedDuration struct {
	duration time.Duration
}

// NewFixedViewDuration returns a ViewDuration with a fixed duration.
func NewFixedDuration(duration time.Duration) *FixedDuration {
	return &FixedDuration{
		duration: duration,
	}
}

// Duration returns the fixed duration.
func (f *FixedDuration) Duration() time.Duration {
	return f.duration
}

// ViewStarted does nothing for FixedViewDuration.
func (f *FixedDuration) ViewStarted() {}

// ViewSucceeded does nothing for FixedViewDuration.
func (f *FixedDuration) ViewSucceeded() {}

// ViewTimeout does nothing for FixedViewDuration.
func (f *FixedDuration) ViewTimeout() {}

var _ ViewDuration = (*FixedDuration)(nil)
