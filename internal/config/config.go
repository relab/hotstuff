package config

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ExperimentConfig holds the configuration for an experiment.
type ExperimentConfig struct {
	// # Host-basCommunicationed configuration values below

	// ReplicaHosts is a list of hosts that will run replicas.
	ReplicaHosts []string
	// ClientHosts is a list of hosts that will run clients.
	ClientHosts []string
	// Replicas is the total number of replicas.
	Replicas int
	// Clients is the total number of clients.
	Clients int
	// Locations is a list of locations for the replicas (optional, but required if TreePositions is set).
	// The length of Locations must be equal to the number of replicas, but it may contain duplicates.
	// The locations are indexed by the replica ID.
	// Entries in Locations must exist in the latency matrix.
	Locations []string
	// ByzantineStrategy is a map from each strategy to a list of replica IDs exhibiting that strategy.
	ByzantineStrategy map[string][]uint32

	// # Tree-based configuration values below:

	// TreePositions is a list of tree positions for the replicas (optional).
	// The length of TreePositions must be equal to the number of replicas and the entries must be unique.
	// The tree positions are indexed by the replica ID.
	// The 0th entry in TreePositions is the tree's root, the 1st entry is the root's left child,
	// the 2nd entry is the root's right child, and so on.
	TreePositions []uint32
	// BranchFactor is the branch factor for the tree (required if TreePositions is set).
	BranchFactor uint32
	// TreeDelta is the waiting time for intermediate nodes in the tree.
	TreeDelta time.Duration
	// Shuffles the tree exisiting positions if true.
	RandomTree bool

	// # Module strings below:

	// Consensus is the name of the consensus implementation to use.
	Consensus string
	// Crypto is the name of the crypto implementation to use.
	Crypto string
	// LeaderRotation is the name of the leader rotation algorithm to use.
	LeaderRotation string
	// Communication is the name of the dissemination and aggregation technique to use.
	Communication string
	// Metrics is a list of metrics to log.
	Metrics []string

	// # Protocol-specific values:

	// UseAggQC indicates whether or not to use aggregated QCs.
	UseAggQC bool
	// Kauri enables Kauri protocol.
	Kauri bool

	// # File path strings below:

	// Cue is the path to optional .cue config file.
	Cue string
	// Exe is the path to the executable deployed to remote hosts.
	Exe string
	// Output is the path to the experiment data output directory.
	Output string
	// SshConfig is the path to the SSH config file.
	SshConfig string

	// # Profiling flags below:

	// CpuProfile enables CPU profiling if true.
	CpuProfile bool
	// FgProfProfile enables fgprof library if true.
	FgProfProfile bool
	// MemProfile enables memory profiling if true.
	MemProfile bool

	// DurationSamples is the number of previous views to consider when predicting view duration.
	DurationSamples uint32

	// # Client-based configuration value below:

	// MaxConcurrent is the maximum number of commands sent concurrently by each client.
	MaxConcurrent uint32
	// PayloadSize is the fixed size, in bytes, of each command sent in a batch by the client.
	PayloadSize uint32
	// RateLimit is the maximum commands per second a client is allowed to send.
	RateLimit float64
	// RateStep is the number of commands per second that the client can increase to up its rate.
	RateStep float64
	// The number of client commands that should be batched together.
	BatchSize uint32

	// # Other values:

	// TimeoutMultiplies is the number to multiply the view duration by in case of a timeout.
	TimeoutMultiplier float64

	// Worker spawns a local worker on the controller.
	Worker bool

	// SharedSeed is the random number generator seed shared across nodes.
	SharedSeed int64
	// Trace enables runtime tracing.
	Trace bool
	// LogLevel is the string specifying the log level.
	LogLevel string
	// UseTLS enables TLS.
	UseTLS bool

	// # Duration values are separated here for easily adding compatibility with Cue.
	// NOTE: Cue does not support time.Duration and must be implemented manually.

	// ClientTimeout specifies the timeout duration of a client.
	ClientTimeout time.Duration
	// ConnectTimeout specifies the duration of the initial connection timeout.
	ConnectTimeout time.Duration
	// RateStepInterval is how often the client rate limit should be increased.
	RateStepInterval time.Duration
	// MeasurementInterval is the time interval between measurements
	MeasurementInterval time.Duration

	// Duration specifies the entire duration of the experiment.
	Duration time.Duration
	// ViewTimeout is the duration of the first view.
	ViewTimeout time.Duration
	// MaxTimeout is the upper limit on view timeouts.
	MaxTimeout time.Duration
}

// TreePosIDs returns a slice of hotstuff.IDs ordered by the tree positions.
func (c *ExperimentConfig) TreePosIDs() []hotstuff.ID {
	ids := make([]hotstuff.ID, 0, len(c.TreePositions))
	for i, id := range c.TreePositions {
		ids[i] = hotstuff.ID(id)
	}
	return ids
}

// ReplicasForHost returns the number of replicas assigned to the host at the given index.
func (c *ExperimentConfig) ReplicasForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Replicas, len(c.ReplicaHosts))
}

// ClientsForHost returns the number of clients assigned to the host at the given index.
func (c *ExperimentConfig) ClientsForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Clients, len(c.ClientHosts))
}

// unitsForHost returns the number of units to be assigned to the host at hostIndex.
func unitsForHost(hostIndex int, totalUnits int, numHosts int) int {
	if numHosts == 0 {
		return 0
	}
	unitsPerHost := totalUnits / numHosts
	remainingUnits := totalUnits % numHosts
	if hostIndex < remainingUnits {
		return unitsPerHost + 1
	}
	return unitsPerHost
}

// AssignReplicas assigns replicas to hosts.
func (c *ExperimentConfig) AssignReplicas(srcReplicaOpts *orchestrationpb.ReplicaOpts) ReplicaMap {
	hostsToReplicas := make(ReplicaMap)
	nextReplicaID := hotstuff.ID(1)

	for hostIdx, host := range c.ReplicaHosts {
		numReplicas := c.ReplicasForHost(hostIdx)
		for range numReplicas {
			replicaOpts := srcReplicaOpts.New(nextReplicaID, c.Locations)
			replicaOpts.SetByzantineStrategy(c.lookupByzStrategy(nextReplicaID))
			hostsToReplicas[host] = append(hostsToReplicas[host], replicaOpts)
			nextReplicaID++
		}
	}
	return hostsToReplicas
}

// lookupByzStrategy returns the Byzantine strategy for the given replica.
// If the replica is not Byzantine, the function will return an empty string.
// This assumes the replicaID is valid; this is checked by the cue config parser.
func (c *ExperimentConfig) lookupByzStrategy(replicaID hotstuff.ID) string {
	for strategy, ids := range c.ByzantineStrategy {
		if slices.Contains(ids, uint32(replicaID)) {
			return strategy
		}
	}
	return ""
}

// AssignClients assigns clients to hosts.
func (c *ExperimentConfig) AssignClients() ClientMap {
	hostsToClients := make(ClientMap)
	nextClientID := hotstuff.ID(1)

	for hostIdx, host := range c.ClientHosts {
		numClients := c.ClientsForHost(hostIdx)
		for range numClients {
			hostsToClients[host] = append(hostsToClients[host], nextClientID)
			nextClientID++
		}
	}
	return hostsToClients
}

// IsLocal returns true if both the replica and client hosts slices
// contain one instance of "localhost". See NewLocal.
func (c *ExperimentConfig) IsLocal() bool {
	if len(c.ClientHosts) > 1 || len(c.ReplicaHosts) > 1 {
		return false
	}
	return c.ReplicaHosts[0] == "localhost" && c.ClientHosts[0] == "localhost" ||
		c.ReplicaHosts[0] == "127.0.0.1" && c.ClientHosts[0] == "127.0.0.1"
}

// AllHosts returns the list of all hostnames, including replicas and clients.
// If the configuration is set to run locally, the function returns a list with
// one entry called "localhost".
func (c *ExperimentConfig) AllHosts() []string {
	if c.IsLocal() {
		return []string{"localhost"}
	}
	return append(c.ReplicaHosts, c.ClientHosts...)
}

// CreateReplicaOpts creates a new ReplicaOpts based on the experiment configuration.
func (c *ExperimentConfig) CreateReplicaOpts() *orchestrationpb.ReplicaOpts {
	return &orchestrationpb.ReplicaOpts{
		UseTLS:            c.UseTLS,
		BatchSize:         c.BatchSize,
		TimeoutMultiplier: float32(c.TimeoutMultiplier),
		Consensus:         c.Consensus,
		Crypto:            c.Crypto,
		LeaderRotation:    c.LeaderRotation,
		Communication:     c.Communication,
		ConnectTimeout:    durationpb.New(c.ConnectTimeout),
		InitialTimeout:    durationpb.New(c.ViewTimeout),
		TimeoutSamples:    c.DurationSamples,
		MaxTimeout:        durationpb.New(c.MaxTimeout),
		SharedSeed:        c.SharedSeed,
		BranchFactor:      c.BranchFactor,
		TreePositions:     c.TreePositions,
		TreeDelta:         durationpb.New(c.TreeDelta),
		Kauri:             c.Kauri,
		UseAggQC:          c.UseAggQC,
	}
}

// CreateClientOpts creates a new ClientOpts based on the experiment configuration.
func (c *ExperimentConfig) CreateClientOpts() *orchestrationpb.ClientOpts {
	return &orchestrationpb.ClientOpts{
		UseTLS:           c.UseTLS,
		ConnectTimeout:   durationpb.New(c.ConnectTimeout),
		PayloadSize:      c.PayloadSize,
		MaxConcurrent:    c.MaxConcurrent,
		RateLimit:        c.RateLimit,
		RateStep:         c.RateStep,
		RateStepInterval: durationpb.New(c.RateStepInterval),
		Timeout:          durationpb.New(c.ClientTimeout),
	}
}

// ReplicaMap maps from a host to a slice of replica options.
type ReplicaMap map[string][]*orchestrationpb.ReplicaOpts

// ReplicaIDs returns the IDs of the replicas running on the given host.
func (r ReplicaMap) ReplicaIDs(host string) []uint32 {
	ids := make([]uint32, 0, len(r[host]))
	for _, opts := range r[host] {
		ids = append(ids, opts.ID)
	}
	return ids
}

// ClientMap maps from a host to a slice of client IDs.
type ClientMap map[string][]hotstuff.ID

// ClientIDs returns the IDs of the clients running on the given host.
func (c ClientMap) ClientIDs(host string) []uint32 {
	ids := make([]uint32, 0, len(c[host]))
	for _, id := range c[host] {
		ids = append(ids, uint32(id))
	}
	return ids
}
