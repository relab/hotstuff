// Package gorums implements a backend for HotStuff using the gorums framework.
package gorums

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type gorumsReplica struct {
	node          *proto.Node
	id            hotstuff.ID
	pubKey        hotstuff.PublicKey
	voteCancel    context.CancelFunc
	newviewCancel context.CancelFunc
}

// ID returns the replica's ID.
func (r *gorumsReplica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *gorumsReplica) PublicKey() hotstuff.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica.
func (r *gorumsReplica) Vote(cert hotstuff.PartialCert) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.voteCancel()
	ctx, r.voteCancel = context.WithCancel(context.Background())
	pCert := proto.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert, gorums.WithNoSendWaiting())
}

// NewView sends the quorum certificate to the other replica.
func (r *gorumsReplica) NewView(msg hotstuff.NewView) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	pQC := proto.QuorumCertToProto(msg.QC)
	r.node.NewView(ctx, &proto.NewViewMsg{View: uint64(msg.View), QC: pQC}, gorums.WithNoSendWaiting())
}

// Deliver sends the block to the other replica
func (r *gorumsReplica) Deliver(block *hotstuff.Block) {
	if r.node == nil {
		return
	}
	// background context is probably fine here, since we are only talking to one replica
	r.node.Deliver(context.Background(), proto.BlockToProto(block), gorums.WithNoSendWaiting())
}

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	replicaCfg    config.ReplicaConfig
	mgr           *proto.Manager
	cfg           *proto.Configuration
	privKey       hotstuff.PrivateKey
	replicas      map[hotstuff.ID]hotstuff.Replica
	proposeCancel context.CancelFunc
}

// NewConfig creates a new configuration.
func NewConfig(replicaCfg config.ReplicaConfig) *Config {
	cfg := &Config{
		replicaCfg:    replicaCfg,
		privKey:       &ecdsa.PrivateKey{PrivateKey: replicaCfg.PrivateKey},
		replicas:      make(map[hotstuff.ID]hotstuff.Replica),
		proposeCancel: func() {},
	}

	for id, r := range replicaCfg.Replicas {
		cfg.replicas[id] = &gorumsReplica{
			id:            r.ID,
			pubKey:        r.PubKey,
			voteCancel:    func() {},
			newviewCancel: func() {},
		}
	}

	return cfg
}

// Connect opens connections to the replicas in the configuration.
func (cfg *Config) Connect(connectTimeout time.Duration) error {
	idMapping := make(map[string]uint32, len(cfg.replicaCfg.Replicas)-1)
	for _, replica := range cfg.replicaCfg.Replicas {
		if replica.ID != cfg.replicaCfg.ID {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", cfg.replicaCfg.ID),
	})

	mgrOpts := []gorums.ManagerOption{
		gorums.WithDialTimeout(connectTimeout),
		gorums.WithMetadata(md),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	}

	if cfg.replicaCfg.Creds != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(cfg.replicaCfg.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

	var err error
	cfg.mgr = proto.NewManager(mgrOpts...)

	cfg.cfg, err = cfg.mgr.NewConfiguration(struct{}{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}
	for _, node := range cfg.cfg.Nodes() {
		id := hotstuff.ID(node.ID())
		cfg.replicas[id].(*gorumsReplica).node = node
	}

	return nil
}

// ID returns the id of this replica.
func (cfg *Config) ID() hotstuff.ID {
	return cfg.replicaCfg.ID
}

// PrivateKey returns the id of this replica.
func (cfg *Config) PrivateKey() hotstuff.PrivateKey {
	return cfg.privKey
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]hotstuff.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica hotstuff.Replica, ok bool) {
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
func (cfg *Config) Propose(block *hotstuff.Block) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	pBlock := proto.BlockToProto(block)
	cfg.cfg.Propose(ctx, pBlock, gorums.WithNoSendWaiting())
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *Config) Fetch(ctx context.Context, hash hotstuff.Hash) {
	cfg.cfg.Fetch(ctx, &proto.BlockHash{Hash: hash[:]}, gorums.WithNoSendWaiting())
}

// Close closes all connections made by this configuration.
func (cfg *Config) Close() {
	cfg.mgr.Close()
}
