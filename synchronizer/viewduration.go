package synchronizer

import (
	"math"
	"time"

	"github.com/relab/hotstuff/consensus"
)

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

// NewViewDuration returns a ViewDuration that approximates the view duration based on durations of previous views.
// sampleSize determines the number of previous views that should be considered.
// startTimeout determines the view duration of the first views.
// When a timeout occurs, the next view duration will be multiplied by the multiplier.
func NewViewDuration(sampleSize uint64, startTimeout, maxTimeout, multiplier float64) ViewDuration {
	return &viewDuration{
		limit: sampleSize,
		mean:  startTimeout,
		max:   maxTimeout,
		mul:   multiplier,
	}
}

// viewDuration uses statistics from previous views to guess a good value for the view duration.
// It only takes a limited amount of measurements into account.
type viewDuration struct {
	mods      *consensus.Modules
	mul       float64   // on failed views, multiply the current mean by this number (should be > 1)
	limit     uint64    // how many measurements should be included in mean
	count     uint64    // total number of measurements
	startTime time.Time // the start time for the current measurement
	mean      float64   // the mean view duration
	m2        float64   // sum of squares of differences from the mean
	prevM2    float64   // m2 calculated from the last period
	max       float64   // upper bound on view timeout
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (v *viewDuration) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	v.mods = mods
}

// ViewSucceeded calculates the duration of the view
// and updates the internal values used for mean and variance calculations.
func (v *viewDuration) ViewSucceeded() {
	if v.startTime.IsZero() {
		return
	}

	duration := float64(time.Since(v.startTime)) / float64(time.Millisecond)
	v.count++

	// Reset m2 occasionally such that we will pick up on changes in variance faster.
	// We store the m2 to prevM2, which will be used when calculating the variance.
	// This ensures that at least 'limit' measurements have contributed to the approximate variance.
	if v.count%v.limit == 0 {
		v.prevM2 = v.m2
		v.m2 = 0
	}

	var c float64
	if v.count > v.limit {
		c = float64(v.limit)
		// throw away one measurement
		v.mean -= v.mean / float64(c)
	} else {
		c = float64(v.count)
	}

	// Welford's algorithm
	d1 := duration - v.mean
	v.mean += d1 / c
	d2 := duration - v.mean
	v.m2 += d1 * d2
}

// ViewTimeout should be called when a view timeout occurred. It will multiply the current mean by 'mul'.
func (v *viewDuration) ViewTimeout() {
	v.mean *= v.mul
}

// ViewStarted records the start time of a view.
func (v *viewDuration) ViewStarted() {
	v.startTime = time.Now()
}

// Duration returns the upper bound of the 95% confidence interval for the mean view duration.
func (v *viewDuration) Duration() time.Duration {
	conf := 1.96 // 95% confidence
	dev := float64(0)
	if v.count > 1 {
		c := float64(v.count)
		m2 := v.m2
		// The standard deviation is calculated from the sum of prevM2 and m2.
		if v.count >= v.limit {
			c = float64(v.limit) + float64(v.count%v.limit)
			m2 += v.prevM2
		}
		dev = math.Sqrt(m2 / c)
	}

	duration := v.mean + dev*conf
	if v.max > 0 && duration > v.max {
		duration = v.max
	}

	if uint64(v.mods.Synchronizer().View())%v.limit == 0 {
		v.mods.Logger().Infof("Mean: %.2fms, Dev: %.2f, Timeout: %.2fms (last %d views)", v.mean, dev, duration, v.limit)
	}
	return time.Duration(duration * float64(time.Millisecond))
}
