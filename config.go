package hotstuff

// Config stores runtime configuration settings.
type Config struct {
	shouldUseAggQC bool
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (c Config) ShouldUseAggQC() bool {
	return c.shouldUseAggQC
}

// ConfigBuilder is used to set the values of immutable configuration settings.
type ConfigBuilder struct {
	cfg Config
}

// SetShouldUseAggQC sets the ShouldUseAggQC setting to true.
func (builder *ConfigBuilder) SetShouldUseAggQC() {
	builder.cfg.shouldUseAggQC = true
}
