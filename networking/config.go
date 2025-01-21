package networking

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/synctools"

	"github.com/relab/hotstuff/logging"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// Replica provides methods used by hotstuff to send messages to replicas.
type Replica struct {
	eventLoop *core.EventLoop
	node      *hotstuffpb.Node
	id        hotstuff.ID
	pubKey    hotstuff.PublicKey
	md        map[string]string
}

// ID returns the replica's ID.
func (r *Replica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *Replica) PublicKey() hotstuff.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica.
func (r *Replica) Vote(cert hotstuff.PartialCert) {
	if r.node == nil {
		return
	}
	ctx, cancel := synctools.TimeoutContext(r.eventLoop.Context(), r.eventLoop)
	defer cancel()
	pCert := hotstuffpb.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert)
}

// NewView sends the quorum certificate to the other replica.
func (r *Replica) NewView(msg hotstuff.SyncInfo) {
	if r.node == nil {
		return
	}
	ctx, cancel := synctools.TimeoutContext(r.eventLoop.Context(), r.eventLoop)
	defer cancel()
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg))
}

// Metadata returns the gRPC metadata from this replica's connection.
func (r *Replica) Metadata() map[string]string {
	return r.md
}

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	opts      []gorums.ManagerOption
	connected bool

	mgr *hotstuffpb.Manager
	SubConfig
}

type SubConfig struct {
	eventLoop *core.EventLoop
	logger    logging.Logger
	opts      *core.Options

	cfg      *hotstuffpb.Configuration
	replicas map[hotstuff.ID]core.Replica
}

// InitModule initializes the configuration.
func (cfg *Config) InitModule(mods *core.Core) {
	mods.Get(
		&cfg.eventLoop,
		&cfg.logger,
		&cfg.SubConfig.opts,
	)

	// We delay processing `replicaConnected` events until after the configurations `connected` event has occurred.
	cfg.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(event any) {
		if !cfg.connected {
			cfg.eventLoop.DelayUntil(ConnectedEvent{}, event)
			return
		}
		cfg.replicaConnected(event.(hotstuff.ReplicaConnectedEvent))
	})
}

// NewConfig creates a new configuration.
func NewConfig(creds credentials.TransportCredentials, opts ...gorums.ManagerOption) *Config {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	// initialization will be finished by InitModule
	cfg := &Config{
		SubConfig: SubConfig{
			replicas: make(map[hotstuff.ID]core.Replica),
		},
		opts: opts,
	}
	return cfg
}

func (cfg *Config) replicaConnected(c hotstuff.ReplicaConnectedEvent) {
	info, peerok := peer.FromContext(c.Ctx)
	md, mdok := metadata.FromIncomingContext(c.Ctx)
	if !peerok || !mdok {
		return
	}

	id, err := GetPeerIDFromContext(c.Ctx, cfg)
	if err != nil {
		cfg.logger.Warnf("Failed to get id for %v: %v", info.Addr, err)
		return
	}

	replica, ok := cfg.replicas[id]
	if !ok {
		cfg.logger.Warnf("Replica with id %d was not found", id)
		return
	}

	replica.(*Replica).md = readMetadata(md)

	cfg.logger.Debugf("Replica %d connected from address %v", id, info.Addr)
}

const keyPrefix = "hotstuff-"

func mapToMetadata(m map[string]string) metadata.MD {
	md := metadata.New(nil)
	for k, v := range m {
		md.Set(keyPrefix+k, v)
	}
	return md
}

func readMetadata(md metadata.MD) map[string]string {
	m := make(map[string]string)
	for k, values := range md {
		if _, key, ok := strings.Cut(k, keyPrefix); ok {
			m[key] = values[0]
		}
	}
	return m
}

// GetRawConfiguration returns the underlying gorums RawConfiguration.
func (cfg *Config) GetRawConfiguration() gorums.RawConfiguration {
	return cfg.cfg.RawConfiguration
}

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	ID       hotstuff.ID
	Address  string
	PubKey   hotstuff.PublicKey
	Location string
}

// Connect opens connections to the replicas in the configuration.
func (cfg *Config) Connect(replicas []ReplicaInfo) (err error) {
	opts := cfg.opts
	cfg.opts = nil // options are not needed beyond this point, so we delete them.

	md := mapToMetadata(cfg.SubConfig.opts.ConnectionMetadata())

	// embed own ID to allow other replicas to identify messages from this replica
	md.Set("id", fmt.Sprintf("%d", cfg.SubConfig.opts.ID()))

	opts = append(opts, gorums.WithMetadata(md))

	cfg.mgr = hotstuffpb.NewManager(opts...)

	// set up an ID mapping to give to gorums
	idMapping := make(map[string]uint32, len(replicas))
	for _, replica := range replicas {
		// also initialize Replica structures
		cfg.replicas[replica.ID] = &Replica{
			eventLoop: cfg.eventLoop,
			id:        replica.ID,
			pubKey:    replica.PubKey,
			md:        make(map[string]string),
		}
		// we do not want to connect to ourself
		if replica.ID != cfg.SubConfig.opts.ID() {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// this will connect to the replicas
	cfg.cfg, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	// now we need to update the "node" field of each replica we connected to
	for _, node := range cfg.cfg.Nodes() {
		// the node ID should correspond with the replica ID
		// because we already configured an ID mapping for gorums to use.
		id := hotstuff.ID(node.ID())
		replica := cfg.replicas[id].(*Replica)
		replica.node = node
	}

	cfg.connected = true

	// this event is sent so that any delayed `replicaConnected` events can be processed.
	cfg.eventLoop.AddEvent(ConnectedEvent{})

	return nil
}

// Replicas returns all of the replicas in the configuration.
func (cfg *SubConfig) Replicas() map[hotstuff.ID]core.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *SubConfig) Replica(id hotstuff.ID) (replica core.Replica, ok bool) {
	replica, ok = cfg.replicas[id]
	return
}

// GetSubConfig returns a subconfiguration containing the replicas specified in the ids slice.
func (cfg *Config) GetSubConfig(ids []hotstuff.ID) (sub core.Configuration, err error) {
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
}

func (cfg *SubConfig) GetSubConfig(_ []hotstuff.ID) (_ core.Configuration, err error) {
	return nil, errors.New("not supported")
}

// Len returns the number of replicas in the configuration.
func (cfg *SubConfig) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *SubConfig) QuorumSize() int {
	return hotstuff.QuorumSize(cfg.Len())
}

// Propose sends the block to all replicas in the configuration
func (cfg *SubConfig) Propose(proposal hotstuff.ProposeMsg) {
	if cfg.cfg == nil {
		return
	}
	ctx, cancel := synctools.TimeoutContext(cfg.eventLoop.Context(), cfg.eventLoop)
	defer cancel()
	cfg.cfg.Propose(
		ctx,
		hotstuffpb.ProposalToProto(proposal),
	)
}

// Timeout sends the timeout message to all replicas.
func (cfg *SubConfig) Timeout(msg hotstuff.TimeoutMsg) {
	if cfg.cfg == nil {
		return
	}

	// will wait until the second timeout before canceling
	ctx, cancel := synctools.TimeoutContext(cfg.eventLoop.Context(), cfg.eventLoop)
	defer cancel()

	cfg.cfg.Timeout(
		ctx,
		hotstuffpb.TimeoutMsgToProto(msg),
	)
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *SubConfig) Fetch(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	protoBlock, err := cfg.cfg.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			cfg.logger.Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

// Close closes all connections made by this configuration.
func (cfg *Config) Close() {
	cfg.mgr.Close()
}

var _ core.Configuration = (*Config)(nil)

type qspec struct{}

// FetchQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) FetchQF(in *hotstuffpb.BlockHash, replies map[uint32]*hotstuffpb.Block) (*hotstuffpb.Block, bool) {
	var h hotstuff.Hash
	copy(h[:], in.GetHash())
	for _, b := range replies {
		block := hotstuffpb.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}

// ConnectedEvent is sent when the configuration has connected to the other replicas.
type ConnectedEvent struct{}
