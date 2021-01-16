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

type gorumsConfig struct {
	mgr           *proto.Manager
	cfg           *proto.Configuration
	id            hotstuff.ID
	privKey       hotstuff.PrivateKey
	replicas      map[hotstuff.ID]gorumsReplica
	proposeCancel context.CancelFunc
}

func NewGorumsConfig(replicaCfg config.ReplicaConfig, connectTimeout time.Duration) (hotstuff.Config, error) {
	cfg := &gorumsConfig{
		id:            replicaCfg.ID,
		privKey:       &ecdsa.PrivateKey{PrivateKey: replicaCfg.PrivateKey},
		replicas:      make(map[hotstuff.ID]gorumsReplica),
		proposeCancel: func() {},
	}

	idMapping := make(map[string]uint32, len(replicaCfg.Replicas)-1)
	for _, replica := range replicaCfg.Replicas {
		if replica.ID != replicaCfg.ID {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", replicaCfg.ID),
	})

	mgrOpts := []gorums.ManagerOption{
		gorums.WithDialTimeout(connectTimeout),
		gorums.WithNodeMap(idMapping),
		gorums.WithMetadata(md),
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithReturnConnectionError(),
	}

	if replicaCfg.Creds != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(replicaCfg.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

	var err error
	cfg.mgr, err = proto.NewManager(mgrOpts...)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to replicas: %w", err)
	}

	for _, node := range cfg.mgr.Nodes() {
		id := hotstuff.ID(node.ID())
		cfg.replicas[id] = gorumsReplica{
			node:          node,
			id:            id,
			pubKey:        replicaCfg.Replicas[id].PubKey,
			voteCancel:    func() {},
			newviewCancel: func() {},
		}
	}

	cfg.cfg, err = cfg.mgr.NewConfiguration(cfg.mgr.NodeIDs(), struct{}{})
	if err != nil {
		return nil, fmt.Errorf("Failed to create configuration: %w", err)
	}

	return cfg, nil
}

// ID returns the id of this replica
func (cfg *gorumsConfig) ID() hotstuff.ID {
	return cfg.id
}

// PrivateKey returns the id of this replica
func (cfg *gorumsConfig) PrivateKey() hotstuff.PrivateKey {
	return cfg.privKey
}

// Replicas returns all of the replicas in the configuration
func (cfg *gorumsConfig) Replicas() map[hotstuff.ID]hotstuff.Replica {
	return cfg.Replicas()
}

// QuorumSize returns the size of a quorum
func (cfg *gorumsConfig) QuorumSize() int {
	return len(cfg.replicas) - (len(cfg.replicas)-1)/3
}

// Propose sends the block to all replicas in the configuration
func (cfg *gorumsConfig) Propose(block hotstuff.Block) {
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	pblock := proto.BlockToProto(block)
	cfg.cfg.Propose(ctx, pblock, gorums.WithAsyncSend())
}

func (cfg *gorumsConfig) Close() {
	cfg.mgr.Close()
}
