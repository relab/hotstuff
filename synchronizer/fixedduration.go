package synchronizer

import "time"

type fixedViewDuration struct {
	duration time.Duration
}

func NewFixedDuration(duration time.Duration) ViewDuration {
	return &fixedViewDuration{duration: duration}
}

// Duration returns the duration that the next view should last.
func (f *fixedViewDuration) Duration() time.Duration {
	return f.duration
}

// ViewStarted is called by the synchronizer when starting a new view.
func (_ *fixedViewDuration) ViewStarted() {

}

// ViewSucceeded is called by the synchronizer when a view ended successfully.
func (_ *fixedViewDuration) ViewSucceeded() {

}

// ViewTimeout is called by the synchronizer when a view timed out.
func (_ *fixedViewDuration) ViewTimeout() {

}
