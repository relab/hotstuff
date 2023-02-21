package modules

import "github.com/relab/hotstuff"

// Options stores runtime configuration settings.
type Options struct {
	id         hotstuff.ID
	privateKey hotstuff.PrivateKey

	shouldUseAggQC        bool
	shouldUseHandel       bool
	shouldVerifyVotesSync bool

	sharedRandomSeed       int64
	connectionMetadata     map[string]string
	shouldCalculateLatency bool
}

// ID returns the ID.
func (opts Options) ID() hotstuff.ID {
	return opts.id
}

// PrivateKey returns the private key.
func (opts Options) PrivateKey() hotstuff.PrivateKey {
	return opts.privateKey
}

// ShouldUseAggQC returns true if aggregated quorum certificates should be used.
// This is true for Fast-Hotstuff: https://arxiv.org/abs/2010.11454
func (opts Options) ShouldUseAggQC() bool {
	return opts.shouldUseAggQC
}

// ShouldUseHandel returns true if the Handel signature aggregation protocol should be used.
func (opts Options) ShouldUseHandel() bool {
	return opts.shouldUseHandel
}

// ShouldVerifyVotesSync returns true if votes should be verified synchronously.
// Enabling this should make the voting machine process votes synchronously.
func (opts Options) ShouldVerifyVotesSync() bool {
	return opts.shouldVerifyVotesSync
}

// SharedRandomSeed returns a random number that is shared between all replicas.
func (opts Options) SharedRandomSeed() int64 {
	return opts.sharedRandomSeed
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (opts Options) ConnectionMetadata() map[string]string {
	return opts.connectionMetadata
}

// SetShouldUseAggQC sets the ShouldUseAggQC setting to true.
func (opts *Options) SetShouldUseAggQC() {
	opts.shouldUseAggQC = true
}

// SetShouldUseHandel sets the ShouldUseHandel setting to true.
func (opts *Options) SetShouldUseHandel() {
	opts.shouldUseHandel = true
}

// IsLatencyCalculationEnabled returns true if the latency calculation is enabled.
func (opts *Options) IsLatencyCalculationEnabled() bool {
	return opts.shouldCalculateLatency
}

// SetLatencyCalculation enables the latency calculations.
func (opts *Options) SetLatencyCalculation(enable bool) {
	opts.shouldCalculateLatency = enable
}

// SetShouldVerifyVotesSync sets the ShouldVerifyVotesSync setting to true.
func (opts *Options) SetShouldVerifyVotesSync() {
	opts.shouldVerifyVotesSync = true
}

// SetSharedRandomSeed sets the shared random seed.
func (opts *Options) SetSharedRandomSeed(seed int64) {
	opts.sharedRandomSeed = seed
}

// SetConnectionMetadata sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func (opts *Options) SetConnectionMetadata(key string, value string) {
	opts.connectionMetadata[key] = value
}
