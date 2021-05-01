package synchronizer

import (
	"math"
	"time"

	"github.com/relab/hotstuff"
)

// viewDuration uses statistics from previous views to guess a good value for the view duration.
type viewDuration struct {
	mod          *hotstuff.HotStuff
	limit        uint64
	count        uint64
	startTime    time.Time // the start time for the current measurement
	mean         float64   // the mean view duration
	m2           float64   // sum of squares of differences from the mean
	lastPeriodM2 float64   // m2 calculated from the last period
}

// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
// settings using the ConfigBuilder.
func (v *viewDuration) InitModule(hs *hotstuff.HotStuff, _ *hotstuff.OptionsBuilder) {
	v.mod = hs
}

func (v *viewDuration) stopTimer() {
	if v.startTime.IsZero() {
		return
	}

	duration := float64(time.Since(v.startTime)) / float64(time.Millisecond)
	v.count++

	// reset m2 occasionally
	if v.count%v.limit == 0 {
		v.lastPeriodM2 = v.m2
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

func (v *viewDuration) startTimer() {
	v.startTime = time.Now()
}

// timeout returns the upper bound of the 95% confidence interval for the mean view duration.
func (v *viewDuration) timeout() float64 {
	conf := 1.96 // 95% confidence
	dev := float64(0)
	if v.count > 1 {
		c := float64(v.count)
		m2 := v.m2
		if v.count >= v.limit {
			c += float64(v.limit)
			m2 += v.lastPeriodM2
		}
		dev = math.Sqrt(m2 / c)
	}
	base := v.mean + dev*conf
	if v.count%v.limit == 0 {
		v.mod.Logger().Infof("last %d views: Mean: %.2fms, Std.dev: %.2f, 95%% conf: %.2f", v.limit, v.mean, dev, base)
	}
	return base
}
