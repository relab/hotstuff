// Package gorums implements a networking backend for HotStuff using the gorums framework.
//
// In particular, this package implements the Configuration and Replica interfaces which are used to send messages.
// This package also receives messages from other replicas and posts them to the event loop.
package gorums

import (
	"context"
	"errors"
	"fmt"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type gorumsReplica struct {
	node          *hotstuffpb.Node
	id            hotstuff.ID
	pubKey        consensus.PublicKey
	voteCancel    context.CancelFunc
	newviewCancel context.CancelFunc
	reputation uint64
	
}

// ID returns the replica's ID.
func (r *gorumsReplica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *gorumsReplica) PublicKey() consensus.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica.
func (r *gorumsReplica) Vote(cert consensus.PartialCert) {
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
func (r *gorumsReplica) NewView(msg consensus.SyncInfo) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg), gorums.WithNoSendWaiting())
}

func (r *gorumsReplica) UpdateRep(rep uint64){
	prevRep := r.GetRep()
	updated := prevRep + rep
	r.reputation = updated
}

func (r *gorumsReplica) GetRep() uint64{
	return r.reputation
}
// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	mods *consensus.Modules

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
}

// NewConfig creates a new configuration.
func NewConfig(id hotstuff.ID, creds credentials.TransportCredentials, opts ...gorums.ManagerOption) *Config {
	cfg := &Config{
		replicas:      make(map[hotstuff.ID]consensus.Replica),
		proposeCancel: func() {},
		timeoutCancel: func() {},
	}
	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", id),
	})

	opts = append(opts, gorums.WithMetadata(md))
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	}

	if creds == nil {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
	}

	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	cfg.mgr = hotstuffpb.NewManager(opts...)
	return cfg
}

// Connect opens connections to the replicas in the configuration.
func (cfg *Config) Connect(replicaCfg *config.ReplicaConfig) (err error) {
	idMapping := make(map[string]uint32, len(replicaCfg.Replicas)-1)
	for _, replica := range replicaCfg.Replicas {
		cfg.replicas[replica.ID] = &gorumsReplica{
			id:            replica.ID,
			pubKey:        replica.PubKey,
			newviewCancel: func() {},
			voteCancel:    func() {},
		}
		if replica.ID != replicaCfg.ID {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	cfg.cfg, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	for _, node := range cfg.cfg.Nodes() {
		id := hotstuff.ID(node.ID())
		replica := cfg.replicas[id].(*gorumsReplica)
		replica.node = node
		//reputation := uint64(0)
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
	if err != nil && !errors.Is(err, context.Canceled) {
		cfg.mods.Logger().Infof("Failed to fetch block: %v", err)
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