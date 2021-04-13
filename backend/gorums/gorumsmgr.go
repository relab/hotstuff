// Package gorums implements a backend for HotStuff using the gorums framework.
package gorums

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
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
func (r *gorumsReplica) NewView(msg hotstuff.SyncInfo) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	r.node.NewView(ctx, proto.SyncInfoToProto(msg), gorums.WithNoSendWaiting())
}

// Manager holds information about the current configuration of replicas that participate in the protocol.
// It provides methods to send messages to the other replicas.
type Manager struct {
	mod *hotstuff.HotStuff

	replicaCfg    config.ReplicaConfig
	mgr           *proto.Manager
	cfg           *proto.Configuration
	privKey       hotstuff.PrivateKey
	replicas      map[hotstuff.ID]hotstuff.Replica
	proposeCancel context.CancelFunc
	timeoutCancel context.CancelFunc
}

// InitModule gives the module a reference to the HotStuff object.
func (cfg *Manager) InitModule(hs *hotstuff.HotStuff) {
	cfg.mod = hs
}

// NewManager creates a new manager.
func NewManager(replicaCfg config.ReplicaConfig) *Manager {
	cfg := &Manager{
		replicaCfg:    replicaCfg,
		privKey:       replicaCfg.PrivateKey,
		replicas:      make(map[hotstuff.ID]hotstuff.Replica),
		proposeCancel: func() {},
		timeoutCancel: func() {},
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
func (cfg *Manager) Connect(connectTimeout time.Duration) error {
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

	cfg.cfg, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	for _, node := range cfg.cfg.Nodes() {
		id := hotstuff.ID(node.ID())
		replica := cfg.replicas[id].(*gorumsReplica)
		replica.node = node
	}

	return nil
}

// ID returns the id of this replica.
func (cfg *Manager) ID() hotstuff.ID {
	return cfg.replicaCfg.ID
}

// PrivateKey returns the id of this replica.
func (cfg *Manager) PrivateKey() hotstuff.PrivateKey {
	return cfg.privKey
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Manager) Replicas() map[hotstuff.ID]hotstuff.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Manager) Replica(id hotstuff.ID) (replica hotstuff.Replica, ok bool) {
	replica, ok = cfg.replicas[id]
	return
}

// Len returns the number of replicas in the configuration.
func (cfg *Manager) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *Manager) QuorumSize() int {
	return hotstuff.QuorumSize(cfg.Len())
}

// Propose sends the block to all replicas in the configuration
func (cfg *Manager) Propose(block *hotstuff.Block) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	pBlock := proto.BlockToProto(block)
	cfg.cfg.Propose(ctx, pBlock, gorums.WithNoSendWaiting())
}

// Timeout sends the timeout message to all replicas.
func (cfg *Manager) Timeout(msg hotstuff.TimeoutMsg) {
	if cfg.cfg == nil {
		return
	}
	var ctx context.Context
	cfg.timeoutCancel()
	ctx, cfg.timeoutCancel = context.WithCancel(context.Background())
	cfg.cfg.Timeout(ctx, proto.TimeoutMsgToProto(msg), gorums.WithNoSendWaiting())
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *Manager) Fetch(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	protoBlock, err := cfg.cfg.Fetch(ctx, &proto.BlockHash{Hash: hash[:]})
	if err != nil && !errors.Is(err, context.Canceled) {
		cfg.mod.Logger().Infof("Failed to fetch block: %v", err)
		return nil, false
	}
	return proto.BlockFromProto(protoBlock), true
}

// Close closes all connections made by this configuration.
func (cfg *Manager) Close() {
	cfg.mgr.Close()
}

var _ hotstuff.Manager = (*Manager)(nil)

type qspec struct{}

// FetchQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) FetchQF(in *proto.BlockHash, replies map[uint32]*proto.Block) (*proto.Block, bool) {
	var h hotstuff.Hash
	copy(h[:], in.GetHash())
	for _, b := range replies {
		block := proto.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}
