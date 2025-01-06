package config

import (
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

// Config holds the configuration for an experiment.
type Config struct {
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
func (c *Config) ReplicasForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Replicas, len(c.ReplicaHosts))
}

// ClientsForHost returns the number of clients assigned to the host at the given index.
func (c *Config) ClientsForHost(hostIndex int) int {
	return unitsForHost(hostIndex, c.Clients, len(c.ClientHosts))
}

// ReplicaMap is a map from host to a list of replica options.
type ReplicaMap map[string][]*orchestrationpb.ReplicaOpts

// AssignReplicas assigns replicas to hosts.
func (c *Config) AssignReplicas(srcReplicaOpts *orchestrationpb.ReplicaOpts) ReplicaMap {
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
func (c *Config) lookupByzStrategy(replicaID hotstuff.ID) string {
	for strategy, ids := range c.ByzantineStrategy {
		if slices.Contains(ids, uint32(replicaID)) {
			return strategy
		}
	}
	return ""
}

// AssignClients assigns clients to hosts.
func (c *Config) AssignClients() map[string][]hotstuff.ID {
	hostsToClients := make(map[string][]hotstuff.ID)
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
