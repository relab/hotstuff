package synchronizer

import "time"

type fixedViewDuration struct {
}

func NewFixedDuration() ViewDuration {
	return &fixedViewDuration{}
}

// Duration returns the duration that the next view should last.
func (_ *fixedViewDuration) Duration() time.Duration {
	return 80 * time.Millisecond
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
