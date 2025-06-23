package viewduration

import "time"

// ViewDuration determines the duration of a view.
// The view synchronizer uses this interface to set its timeouts.
type ViewDuration interface {
	// Duration returns the duration that the next view should last.
	Duration() time.Duration
	// ViewStarted is called by the synchronizer when starting a new view.
	ViewStarted()
	// ViewSucceeded is called by the synchronizer when a view ended successfully.
	ViewSucceeded()
	// ViewTimeout is called by the synchronizer when a view timed out.
	ViewTimeout()
}
