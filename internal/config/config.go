package config

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

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

// HostConfig holds the configuration for an experiment.
type HostConfig struct {
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
	// TreePositions is a list of tree positions for the replicas (optional).
	// The length of TreePositions must be equal to the number of replicas and the entries must be unique.
	// The tree positions are indexed by the replica ID.
	// The 0th entry in TreePositions is the tree's root, the 1st entry is the root's left child,
	// the 2nd entry is the root's right child, and so on.
	TreePositions []uint32
	// BranchFactor is the branch factor for the tree (required if TreePositions is set).
	BranchFactor uint32
	// ByzantineStrategy is a map from each strategy to a list of replica IDs exhibiting that strategy.
	ByzantineStrategy map[string][]uint32
	// TreeDelta is the waiting time for intermediate nodes in the tree.
	TreeDelta time.Duration
}

// NewLocal is a helper that creates a config for local hosting.
func NewLocal(numReplicas, numClients int) *HostConfig {
	return &HostConfig{
		ReplicaHosts: []string{"localhost"},
		ClientHosts:  []string{"localhost"},
		Replicas:     numReplicas,
		Clients:      numClients,
	}
}

// TreePosIDs returns a slice of hotstuff.IDs ordered by the tree positions.
func (c *HostConfig) TreePosIDs() []hotstuff.ID {
	ids := make([]hotstuff.ID, 0, len(c.TreePositions))
	for i, id := range c.TreePositions {
		ids[i] = hotstuff.ID(id)
	}
	return ids
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

// ReplicasForHost returns the number of replicas assigned to the host at the given index.
func (c *HostConfig) ReplicasForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Replicas, len(c.ReplicaHosts))
}

// ClientsForHost returns the number of clients assigned to the host at the given index.
func (c *HostConfig) ClientsForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Clients, len(c.ClientHosts))
}

// AssignReplicas assigns replicas to hosts.
func (c *HostConfig) AssignReplicas(srcReplicaOpts *orchestrationpb.ReplicaOpts) ReplicaMap {
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
func (c *HostConfig) lookupByzStrategy(replicaID hotstuff.ID) string {
	for strategy, ids := range c.ByzantineStrategy {
		if slices.Contains(ids, uint32(replicaID)) {
			return strategy
		}
	}
	return ""
}

// AssignClients assigns clients to hosts.
func (c *HostConfig) AssignClients() ClientMap {
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
func (c *HostConfig) IsLocal() bool {
	if len(c.ClientHosts) > 1 || len(c.ReplicaHosts) > 1 {
		return false
	}
	return c.ReplicaHosts[0] == "localhost" && c.ClientHosts[0] == "localhost" ||
		c.ReplicaHosts[0] == "127.0.0.1" && c.ClientHosts[0] == "127.0.0.1"
}

// AllHosts returns the list of all hostnames, including replicas and clients.
// If the configuration is set to run locally, the function returns a list with
// one entry called "localhost".
func (c *HostConfig) AllHosts() []string {
	if c.IsLocal() {
		return []string{"localhost"}
	}
	return append(c.ReplicaHosts, c.ClientHosts...)
}
