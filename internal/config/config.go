package config

import (
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

type Config struct {
	LatenciesFile     string
	ReplicaHosts      []string
	ClientHosts       []string
	Replicas          int
	Clients           int
	Locations         []string
	TreePositions     []uint32
	BranchFactor      int
	ByzantineStrategy map[string][]uint32
}

func unitsForHost(hostIndex int, numUnits int, numHosts int) int {
	if numHosts == 0 {
		return 0
	}
	unitsPerHost := numUnits / numHosts
	remainingUnits := numUnits % numHosts
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
