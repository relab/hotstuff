package synchronizer

import (
	"math"
	"time"

	"github.com/relab/hotstuff"
)

// viewDuration uses statistics from previous views to guess a good value for the view duration.
// It only takes a limited amount of measurements into account.
type viewDuration struct {
	mod       *hotstuff.HotStuff
	limit     uint64    // how many measurements should be included in mean.
	count     uint64    // total number of measurements
	startTime time.Time // the start time for the current measurement
	mean      float64   // the mean view duration
	m2        float64   // sum of squares of differences from the mean
	prevM2    float64   // m2 calculated from the last period
}

// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
// settings using the ConfigBuilder.
func (v *viewDuration) InitModule(hs *hotstuff.HotStuff, _ *hotstuff.OptionsBuilder) {
	v.mod = hs
}

// stopViewTimer calculates the duration of the view
// and updates the internal values used for mean and variance calculations.
func (v *viewDuration) stopViewTimer() {
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
	v.mean += duration / c
	d2 := duration - v.mean
	v.m2 += d1 * d2
}

// startViewTimer records the start time of a view.
func (v *viewDuration) startViewTimer() {
	v.startTime = time.Now()
}

// timeout returns the upper bound of the 95% confidence interval for the mean view duration.
func (v *viewDuration) timeout() float64 {
	conf := 1.96 // 95% confidence
	dev := float64(0)
	if v.count > 1 {
		c := float64(v.count)
		m2 := v.m2
		// The standard deviation is calculated from the sum of prevM2 and m2.
		if v.count >= v.limit {
			c += float64(v.limit)
			m2 += v.prevM2
		}
		dev = math.Sqrt(m2 / c)
	}
	base := v.mean + dev*conf
	if v.count%v.limit == 0 {
		v.mod.Logger().Infof("last %d views: Mean: %.2fms, Std.dev: %.2f, 95%% conf: %.2f", v.limit, v.mean, dev, base)
	}
	return base
}
