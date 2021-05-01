package synchronizer

import (
	"math"
	"time"
)

// viewDuration uses statistics from previous views to guess a good value for the view duration.
type viewDuration struct {
	count uint64
	mean  float64
	m2    float64

	startTime time.Time
}

func (v *viewDuration) stopTimer() {
	if v.startTime.IsZero() {
		return
	}

	duration := float64(time.Since(v.startTime)) / float64(time.Millisecond)

	// Welford's algorithm
	v.count++
	d1 := duration - v.mean
	v.mean += d1 / float64(v.count)
	d2 := duration - v.mean
	v.m2 += d1 * d2
}

func (v *viewDuration) startTimer() {
	v.startTime = time.Now()
}

// timeout returns the upper bound of the 95% confidence interval for the mean view duration.
func (v *viewDuration) timeout() float64 {
	conf := 1.96 // 95% confidence
	var dev float64
	if v.count > 1 {
		dev = math.Sqrt(v.m2 / (float64(v.count) - 1))
	}
	return v.mean + dev*conf
}
