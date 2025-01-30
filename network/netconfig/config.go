package netconfig

import (
	"github.com/relab/hotstuff"
)

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica.
type Config struct {
	replicaCount int
	replicas     map[hotstuff.ID]*hotstuff.ReplicaInfo
}

// NewConfig creates a new configuration.
func NewConfig() *Config {
	// initialization will be finished by InitModule
	cfg := &Config{
		replicaCount: 0,
		replicas:     make(map[hotstuff.ID]*hotstuff.ReplicaInfo),
	}
	return cfg
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]*hotstuff.ReplicaInfo {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica *hotstuff.ReplicaInfo, ok bool) {
	replica, ok = cfg.replicas[id]
	return
}

// Len returns the number of replicas in the configuration.
func (cfg *Config) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *Config) QuorumSize() int {
	return hotstuff.QuorumSize(cfg.Len())
}

// Custom methods not part of core.Configuration interface.

func (cfg *Config) AddReplica(replicaInfo *hotstuff.ReplicaInfo) {
	cfg.replicaCount++
	cfg.replicas[replicaInfo.ID] = replicaInfo
}

func (cfg *Config) SetReplicaMetaData(id hotstuff.ID, metaData map[string]string) {
	cfg.replicas[id].MetaData = metaData
}
