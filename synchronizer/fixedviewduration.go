package synchronizer

import "time"

type fixedViewDuration struct {
	duration time.Duration
}

func NewFixedViewDuration(duration time.Duration) ViewDuration {
	return &fixedViewDuration{
		duration: duration,
	}
}

func (f *fixedViewDuration) Duration() time.Duration {
	return f.duration
}

// fixed view duration does not need to keep track of anything
func (f *fixedViewDuration) ViewStarted() {}

func (f *fixedViewDuration) ViewSucceeded() {}

func (f *fixedViewDuration) ViewTimeout() {}
