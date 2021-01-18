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

// ID returns the replica's id
func (r *gorumsReplica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key
func (r *gorumsReplica) PublicKey() hotstuff.PublicKey {
	return r.pubKey
}

// Vote sends the partial certificate to the other replica
func (r *gorumsReplica) Vote(cert hotstuff.PartialCert) {
	var ctx context.Context
	r.voteCancel()
	ctx, r.voteCancel = context.WithCancel(context.Background())
	pcert := proto.PartialCertToProto(cert)
	r.node.Vote(ctx, pcert, gorums.WithAsyncSend())
}

// NewView sends the quorum certificate to the other replica
func (r *gorumsReplica) NewView(qc hotstuff.QuorumCert) {
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	pqc := proto.QuorumCertToProto(qc)
	r.node.NewView(ctx, pqc, gorums.WithAsyncSend())
}

type Config struct {
	replicaCfg    config.ReplicaConfig
	mgr           *proto.Manager
	cfg           *proto.Configuration
	privKey       hotstuff.PrivateKey
	replicas      map[hotstuff.ID]hotstuff.Replica
	proposeCancel context.CancelFunc
}

func NewConfig(replicaCfg config.ReplicaConfig, connectTimeout time.Duration) *Config {
	cfg := &Config{
		replicaCfg:    replicaCfg,
		privKey:       &ecdsa.PrivateKey{PrivateKey: replicaCfg.PrivateKey},
		replicas:      make(map[hotstuff.ID]hotstuff.Replica),
		proposeCancel: func() {},
	}

	return cfg
}

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
		gorums.WithNodeMap(idMapping),
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
	cfg.mgr, err = proto.NewManager(mgrOpts...)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}

	for _, node := range cfg.mgr.Nodes() {
		id := hotstuff.ID(node.ID())
		cfg.replicas[id] = &gorumsReplica{
			node:          node,
			id:            id,
			pubKey:        cfg.replicaCfg.Replicas[id].PubKey,
			voteCancel:    func() {},
			newviewCancel: func() {},
		}
	}

	cfg.replicas[cfg.ID()] = &gorumsReplica{
		node:   nil,
		id:     cfg.ID(),
		pubKey: cfg.privKey.PublicKey(),
	}

	cfg.cfg, err = cfg.mgr.NewConfiguration(cfg.mgr.NodeIDs(), struct{}{})
	if err != nil {
		return fmt.Errorf("Failed to create configuration: %w", err)
	}

	return nil
}

// ID returns the id of this replica
func (cfg *Config) ID() hotstuff.ID {
	return cfg.replicaCfg.ID
}

// PrivateKey returns the id of this replica
func (cfg *Config) PrivateKey() hotstuff.PrivateKey {
	return cfg.privKey
}

// Replicas returns all of the replicas in the configuration
func (cfg *Config) Replicas() map[hotstuff.ID]hotstuff.Replica {
	return cfg.replicas
}

// QuorumSize returns the size of a quorum
func (cfg *Config) QuorumSize() int {
	return len(cfg.replicaCfg.Replicas) - (len(cfg.replicaCfg.Replicas)-1)/3
}

// Propose sends the block to all replicas in the configuration
func (cfg *Config) Propose(block hotstuff.Block) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	pblock := proto.BlockToProto(block)
	cfg.cfg.Propose(ctx, pblock, gorums.WithAsyncSend())
}

func (cfg *Config) Close() {
	cfg.mgr.Close()
}
