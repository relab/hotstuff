package netconfig

import (
	"github.com/relab/hotstuff/core"

	"github.com/relab/hotstuff"
)

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica.
type Config struct {
	replicas map[hotstuff.ID]hotstuff.ReplicaInfo
}

// InitModule initializes the configuration.
func (cfg *Config) InitModule(mods *core.Core) {

}

// NewConfig creates a new configuration.
func NewConfig() *Config {
	// initialization will be finished by InitModule
	cfg := &Config{
		replicas: make(map[hotstuff.ID]hotstuff.ReplicaInfo),
	}
	return cfg
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]hotstuff.ReplicaInfo {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica hotstuff.ReplicaInfo, ok bool) {
	r, ok := cfg.replicas[id]
	if !ok {
		return hotstuff.ReplicaInfo{}, false
	}
	return r, true
}

// GetSubConfig returns a subconfiguration containing the replicas specified in the ids slice.
/*func (cfg *Config) GetSubConfig(_ []hotstuff.ID) (core.Configuration, error) {
	return nil, fmt.Errorf("subconfig support removed")
	replicas := make(map[hotstuff.ID]core.Replica)
	nids := make([]uint32, len(ids))
	for i, id := range ids {
		nids[i] = uint32(id)
		replicas[id] = cfg.replicas[id]
	}
	newCfg, err := cfg.mgr.NewConfiguration(gorums.WithNodeIDs(nids))
	if err != nil {
		return nil, err
	}
	return &SubConfig{
		eventLoop: cfg.eventLoop,
		logger:    cfg.logger,
		opts:      cfg.SubConfig.opts,
		cfg:       newCfg,
		replicas:  replicas,
	}, nil
}*/

// Len returns the number of replicas in the configuration.
func (cfg *Config) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *Config) QuorumSize() int {
	return hotstuff.QuorumSize(cfg.Len())
}

// Custom methods not part of core.Configuration interface.

func (cfg *Config) AddReplica(replicaInfo hotstuff.ReplicaInfo) {
	cfg.replicas[replicaInfo.ID] = replicaInfo
}

var _ core.Configuration = (*Config)(nil)
