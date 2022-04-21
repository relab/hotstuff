// Package backend implements the networking backend for hotstuff using the Gorums framework.
package backend

import (
	"context"
	"fmt"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Replica provides methods used by hotstuff to send messages to replicas.
type Replica struct {
	node          *hotstuffpb.Node
	id            hotstuff.ID
	pubKey        consensus.PublicKey
	voteCancel    context.CancelFunc
	newviewCancel context.CancelFunc
}

// ID returns the replica's ID.
func (r *Replica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *Replica) PublicKey() consensus.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica.
func (r *Replica) Vote(cert consensus.PartialCert) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.voteCancel()
	ctx, r.voteCancel = context.WithCancel(context.Background())
	pCert := hotstuffpb.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert, gorums.WithNoSendWaiting())
}

// NewView sends the quorum certificate to the other replica.
func (r *Replica) NewView(msg consensus.SyncInfo) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg), gorums.WithNoSendWaiting())
}

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	mods    *consensus.Modules
	optsPtr *[]gorums.ManagerOption // using a pointer so that options can be GCed after initialization

	mgr           *hotstuffpb.Manager
	cfg           *hotstuffpb.Configuration
	replicas      map[hotstuff.ID]consensus.Replica
	proposeCancel context.CancelFunc
	timeoutCancel context.CancelFunc
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (cfg *Config) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	cfg.mods = mods

	opts := *cfg.optsPtr
	cfg.optsPtr = nil // we don't need to keep the options around beyond this point, so we'll allow them to be GCed.

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", cfg.mods.ID()),
	})

	opts = append(opts, gorums.WithMetadata(md))

	cfg.mgr = hotstuffpb.NewManager(opts...)
}

// NewConfig creates a new configuration.
func NewConfig(creds credentials.TransportCredentials, opts ...gorums.ManagerOption) *Config {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
		grpc.WithTransportCredentials(creds),
	}
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	// initialization will be finished by InitConsensusModule
	cfg := &Config{
		replicas:      make(map[hotstuff.ID]consensus.Replica),
		optsPtr:       &opts,
		proposeCancel: func() {},
		timeoutCancel: func() {},
	}
	return cfg
}

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	ID      hotstuff.ID
	Address string
	PubKey  consensus.PublicKey
}

// Connect opens connections to the replicas in the configuration.
func (cfg *Config) Connect(replicas []ReplicaInfo) (err error) {
	// set up an ID mapping to give to gorums
	idMapping := make(map[string]uint32, len(replicas))
	for _, replica := range replicas {
		// also initialize Replica structures
		cfg.replicas[replica.ID] = &Replica{
			id:            replica.ID,
			pubKey:        replica.PubKey,
			newviewCancel: func() {},
			voteCancel:    func() {},
		}
		// we do not want to connect to ourself
		if replica.ID != cfg.mods.ID() {
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

	return nil
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]consensus.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica consensus.Replica, ok bool) {
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

// Propose sends the block to all replicas in the configuration
func (cfg *Config) Propose(proposal consensus.ProposeMsg) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	p := hotstuffpb.ProposalToProto(proposal)
	cfg.cfg.Propose(ctx, p, gorums.WithNoSendWaiting())
}

// Timeout sends the timeout message to all replicas.
func (cfg *Config) Timeout(msg consensus.TimeoutMsg) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.timeoutCancel()
	ctx, cfg.timeoutCancel = context.WithCancel(context.Background())
	cfg.cfg.Timeout(ctx, hotstuffpb.TimeoutMsgToProto(msg), gorums.WithNoSendWaiting())
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *Config) Fetch(ctx context.Context, hash consensus.Hash) (*consensus.Block, bool) {
	protoBlock, err := cfg.cfg.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			cfg.mods.Logger().Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

// Close closes all connections made by this configuration.
func (cfg *Config) Close() {
	cfg.mgr.Close()
}

var _ consensus.Configuration = (*Config)(nil)

type qspec struct{}

// FetchQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) FetchQF(in *hotstuffpb.BlockHash, replies map[uint32]*hotstuffpb.Block) (*hotstuffpb.Block, bool) {
	var h consensus.Hash
	copy(h[:], in.GetHash())
	for _, b := range replies {
		block := hotstuffpb.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}
