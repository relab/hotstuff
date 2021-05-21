package consensus

// Options stores runtime configuration settings.
type Options struct {
	shouldUseAggQC bool
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (c Options) ShouldUseAggQC() bool {
	return c.shouldUseAggQC
}

// OptionsBuilder is used to set the values of immutable configuration settings.
type OptionsBuilder struct {
	opts Options
}

// SetShouldUseAggQC sets the ShouldUseAggQC setting to true.
func (builder *OptionsBuilder) SetShouldUseAggQC() {
	builder.opts.shouldUseAggQC = true
}
