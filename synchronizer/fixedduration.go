package synchronizer

import "time"

type FixedViewDuration struct {
}

// Duration returns the duration that the next view should last.
func (_ *FixedViewDuration) Duration() time.Duration {
	return 100 * time.Millisecond
}

// ViewStarted is called by the synchronizer when starting a new view.
func (_ *FixedViewDuration) ViewStarted() {

}

// ViewSucceeded is called by the synchronizer when a view ended successfully.
func (_ *FixedViewDuration) ViewSucceeded() {

}

// ViewTimeout is called by the synchronizer when a view timed out.
func (_ *FixedViewDuration) ViewTimeout() {

}
