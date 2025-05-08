package viewduration

// TODO(AlanRostem): make conversion easier and use time.Duration instead of float
type Options struct {
	SampleSize   uint64
	StartTimeout float64
	MaxTimeout   float64
	Multiplier   float64
}
