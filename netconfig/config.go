package netconfig

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"

	"github.com/relab/hotstuff/logging"

	"github.com/relab/hotstuff"
)

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	eventLoop *core.EventLoop
	logger    logging.Logger
	opts      *core.Options

	replicas map[hotstuff.ID]core.Replica
}

// InitModule initializes the configuration.
func (cfg *Config) InitModule(mods *core.Core) {
	mods.Get(
		&cfg.eventLoop,
		&cfg.logger,
		&cfg.opts,
	)
}

// NewConfig creates a new configuration.
func NewConfig() *Config {
	// initialization will be finished by InitModule
	cfg := &Config{
		replicas: make(map[hotstuff.ID]core.Replica),
	}
	return cfg
}

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	ID       hotstuff.ID
	Address  string
	PubKey   hotstuff.PublicKey
	Location string
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]core.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica core.Replica, ok bool) {
	replica, ok = cfg.replicas[id]
	return
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

func (cfg *Config) AddReplica(
	id hotstuff.ID,
	eventLoop *core.EventLoop,
	pubKey hotstuff.PublicKey,
) {
	cfg.replicas[id] = &Replica{
		eventLoop: eventLoop,
		id:        id,
		pubKey:    pubKey,
		md:        make(map[string]string),
	}
}

func (cfg *Config) SetReplicaNode(id hotstuff.ID, node *hotstuffpb.Node) {
	replica := cfg.replicas[id].(*Replica)
	replica.node = node
}

func (cfg *Config) SetReplicaMetaData(id hotstuff.ID, metaData map[string]string) {
	replica := cfg.replicas[id].(*Replica)
	replica.md = metaData
}

var _ core.Configuration = (*Config)(nil)

// ConnectedEvent is sent when the configuration has connected to the other replicas.
type ConnectedEvent struct{}
