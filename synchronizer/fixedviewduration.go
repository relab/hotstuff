package synchronizer

import "time"

type fixedViewDuration struct {
	duration time.Duration
}

// NewFixedViewDuration returns a ViewDuration with a fixed duration.
func NewFixedViewDuration(duration time.Duration) ViewDuration {
	return &fixedViewDuration{
		duration: duration,
	}
}

// Duration returns the fixed duration.
func (f *fixedViewDuration) Duration() time.Duration {
	return f.duration
}

// ViewStarted does nothing for the FixedViewDuration.
func (f *fixedViewDuration) ViewStarted() {}

// ViewSucceeded does nothing for the FixedViewDuration.
func (f *fixedViewDuration) ViewSucceeded() {}

// ViewTimeout does nothing for the FixedViewDuration.
func (f *fixedViewDuration) ViewTimeout() {}
