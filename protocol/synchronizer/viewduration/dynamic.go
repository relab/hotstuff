package viewduration

import (
	"math"
	"time"

	"github.com/relab/hotstuff/modules"
)

// NewDynamic returns a ViewDuration that approximates the view duration based on durations of previous views.
// sampleSize determines the number of previous views that should be considered.
// startTimeout determines the view duration of the first views.
// When a timeout occurs, the next view duration will be multiplied by the multiplier.
func NewDynamic(opt Params) *Dynamic {
	return &Dynamic{
		limit: opt.sampleSize,
		mean:  opt.startTimeout,
		max:   opt.maxTimeout,
		mul:   opt.multiplier,
	}
}

// Dynamic uses statistics from previous views to guess a good value for the view duration.
// It only takes a limited amount of measurements into account.
type Dynamic struct {
	mul       float64   // on failed views, multiply the current mean by this number (should be > 1)
	limit     uint64    // how many measurements should be included in mean
	count     uint64    // total number of measurements
	startTime time.Time // the start time for the current measurement
	mean      float64   // the mean view duration
	m2        float64   // sum of squares of differences from the mean
	prevM2    float64   // m2 calculated from the last period
	max       float64   // upper bound on view timeout
}

// ViewSucceeded calculates the duration of the view
// and updates the internal values used for mean and variance calculations.
func (v *Dynamic) ViewSucceeded() {
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
func (v *Dynamic) ViewTimeout() {
	v.mean *= v.mul
}

// ViewStarted records the start time of a view.
func (v *Dynamic) ViewStarted() {
	v.startTime = time.Now()
}

// Duration returns the upper bound of the 95% confidence interval for the mean view duration.
func (v *Dynamic) Duration() time.Duration {
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

	return time.Duration(duration * float64(time.Millisecond))
}

var _ modules.ViewDuration = (*Dynamic)(nil)
