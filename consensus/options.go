package consensus

// Options stores runtime configuration settings.
type Options struct {
	shouldUseAggQC        bool
	shouldVerifyVotesSync bool

	sharedRandomSeed   int64
	connectionMetadata map[string]string
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (c Options) ShouldUseAggQC() bool {
	return c.shouldUseAggQC
}

// ShouldVerifyVotesSync returns true if votes should be verified synchronously.
// Enabling this should make the voting machine process votes synchronously.
func (c Options) ShouldVerifyVotesSync() bool {
	return c.shouldVerifyVotesSync
}

// SharedRandomSeed returns a random number that is shared between all replicas.
func (c Options) SharedRandomSeed() int64 {
	return c.sharedRandomSeed
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (c Options) ConnectionMetadata() map[string]string {
	return c.connectionMetadata
}

// OptionsBuilder is used to set the values of immutable configuration settings.
type OptionsBuilder struct {
	opts *Options
}

// SetShouldUseAggQC sets the ShouldUseAggQC setting to true.
func (builder *OptionsBuilder) SetShouldUseAggQC() {
	builder.opts.shouldUseAggQC = true
}

// SetShouldVerifyVotesSync sets the ShouldVerifyVotesSync setting to true.
func (builder *OptionsBuilder) SetShouldVerifyVotesSync() {
	builder.opts.shouldVerifyVotesSync = true
}

// SetSharedRandomSeed sets the shared random seed.
func (builder *OptionsBuilder) SetSharedRandomSeed(seed int64) {
	builder.opts.sharedRandomSeed = seed
}

// SetConnectionMetadata sets the value of a key in the connection metadata map.
func (builder *OptionsBuilder) SetConnectionMetadata(key string, value string) {
	builder.opts.connectionMetadata[key] = value
}
