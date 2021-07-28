package metrics

import "math"

// Welford is an implementation of Welford's online algorithm for calculating variance.
type Welford struct {
	mean  float64
	m2    float64
	count uint64
}

// Update adds the value to the current estimate.
func (w *Welford) Update(val float64) {
	w.count++
	delta := val - w.mean
	w.mean += delta / float64(w.count)
	delta2 := val - w.mean
	w.m2 += delta * delta2
}

// Get returns the current mean and sample variance estimate.
func (w *Welford) Get() (mean, variance float64, count uint64) {
	if w.count < 2 {
		return w.mean, math.NaN(), w.count
	}
	return w.mean, w.m2 / (float64(w.count - 1)), w.count
}

// Count returns the total number of values that have been added to the variance estimate.
func (w *Welford) Count() uint64 {
	return w.count
}

// Reset resets all values to 0.
func (w *Welford) Reset() {
	w.mean = 0
	w.m2 = 0
	w.count = 0
}
