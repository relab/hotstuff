package viewduration

import "time"

type Params struct {
	sampleSize   uint64
	startTimeout float64
	MaxTimeout   float64
	Multiplier   float64
}

func NewParams(
	sampleSize uint32,
	startTimeout time.Duration,
	maxTimeout time.Duration,
	multiplier float32,
) Params {
	return Params{
		sampleSize:   uint64(sampleSize),
		startTimeout: float64(startTimeout.Nanoseconds()) / float64(time.Millisecond),
		MaxTimeout:   float64(maxTimeout.Nanoseconds()) / float64(time.Millisecond),
		Multiplier:   float64(multiplier),
	}
}

// StartTimeout returns the initial timeout duration.
func (opt Params) StartTimeout() time.Duration {
	return time.Duration(opt.startTimeout) * time.Millisecond
}
