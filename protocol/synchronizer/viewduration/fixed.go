package viewduration

import (
	"time"

	"github.com/relab/hotstuff/modules"
)

type Fixed struct {
	duration time.Duration
}

// NewFixedViewDuration returns a ViewDuration with a fixed duration.
func NewFixed(duration time.Duration) *Fixed {
	return &Fixed{
		duration: duration,
	}
}

// Duration returns the fixed duration.
func (f *Fixed) Duration() time.Duration {
	return f.duration
}

// ViewStarted does nothing for FixedViewDuration.
func (f *Fixed) ViewStarted() {}

// ViewSucceeded does nothing for FixedViewDuration.
func (f *Fixed) ViewSucceeded() {}

// ViewTimeout does nothing for FixedViewDuration.
func (f *Fixed) ViewTimeout() {}

var _ modules.ViewDuration = (*Fixed)(nil)
