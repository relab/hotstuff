package config

import (
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

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
	BranchFactor int
	// ByzantineStrategy is a map from each strategy to a list of replica IDs exhibiting that strategy.
	ByzantineStrategy map[string][]uint32
}

// NewLocal creates a config for a localhost case.
// TODO: Add support for locations through cli.
func NewLocal(numReplicas, numClients int,

// locations []string,
// byzStrat map[string][]uint32,
) *HostConfig {
	return &HostConfig{
		ReplicaHosts: []string{"localhost"},
		ClientHosts:  []string{"localhost"},
		Replicas:     numReplicas,
		Clients:      numClients,
		// Locations:         locations,
		// ByzantineStrategy: byzStrat,
	}
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

// ReplicaMap is a map from host to a list of replica options.
type ReplicaMap map[string][]*orchestrationpb.ReplicaOpts

// ReplicaIDs convert the existing map to a map of []uint32.
func (r ReplicaMap) ReplicaIDs(host string) []uint32 {
	ids := make([]uint32, 0, len(r[host]))
	for _, opts := range r[host] {
		ids = append(ids, opts.ID)
	}
	return ids
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

type ClientMap map[string][]hotstuff.ID

// ClientIDs convert the existing map from hotstuff.ID to uint32.
func (c ClientMap) ClientIDs(host string) []uint32 {
	newList := make([]uint32, 0, len(c))
	for _, id := range c[host] {
		newList = append(newList, uint32(id))
	}
	return newList
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
// contain one instance of "localhost".
func (c *HostConfig) isLocal() bool {
	if len(c.ClientHosts) > 1 || len(c.ReplicaHosts) > 1 {
		return false
	}
	return c.ReplicaHosts[0] == "localhost" && c.ClientHosts[0] == "localhost" ||
		c.ReplicaHosts[0] == "127.0.0.1" && c.ClientHosts[0] == "127.0.0.1"
}

// AllHosts returns the list of all hostnames, including replicas and clients.
// If the configuration is set to run locally, the function returns an empty list.
func (c *HostConfig) AllHosts() []string {
	if c.isLocal() {
		return []string{}
	}
	return append(c.ReplicaHosts, c.ClientHosts...)
}
