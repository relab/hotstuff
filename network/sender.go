package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type Sender struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	mgrOpts   []gorums.ManagerOption
	connected bool

	mgr      *hotstuffpb.Manager
	replicas map[hotstuff.ID]*Replica

	pbCfg *hotstuffpb.Configuration
}

func NewSender(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption) *Sender {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}
	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

	inv := &Sender{
		eventLoop: eventLoop,
		logger:    logger,
		config:    config,

		mgrOpts:  mgrOpts,
		replicas: make(map[hotstuff.ID]*Replica),
	}

	// We delay processing `replicaConnected` events until after the configurations `connected` event has occurred.
	inv.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(event any) {
		if !inv.connected {
			inv.eventLoop.DelayUntil(ConnectedEvent{}, event)
			return
		}
		inv.replicaConnected(event.(hotstuff.ReplicaConnectedEvent))
	})
	return inv
}

func (inv *Sender) Connect(replicas []hotstuff.ReplicaInfo) (err error) {
	mgrOpts := inv.mgrOpts
	// TODO(AlanRostem): this was here when the subConfig pattern was in use. Check if doing this is valid
	// cfg.opts = nil // options are not needed beyond this point, so we delete them.

	md := mapToMetadata(inv.config.ConnectionMetadata())

	// embed own ID to allow other replicas to identify messages from this replica
	md.Set("id", fmt.Sprintf("%d", inv.config.ID()))

	mgrOpts = append(mgrOpts, gorums.WithMetadata(md))

	inv.mgr = hotstuffpb.NewManager(mgrOpts...)

	// set up an ID mapping to give to gorums
	idMapping := make(map[string]uint32, len(replicas))
	for _, replica := range replicas {
		// also initialize Replica structures
		realReplica := &Replica{
			eventLoop: inv.eventLoop,
			id:        replica.ID,
			pubKey:    replica.PubKey,
			// node and metaData is set later
		}
		inv.replicas[replica.ID] = realReplica
		// add the info to the config
		inv.config.AddReplica(&replica)
		// we do not want to connect to ourself
		if replica.ID != inv.config.ID() {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}

	// this will connect to the replicas
	inv.pbCfg, err = inv.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	// now we need to update the "node" field of each replica we connected to
	for _, node := range inv.pbCfg.Nodes() {
		// the node ID should correspond with the replica ID
		// because we already configured an ID mapping for gorums to use.
		id := hotstuff.ID(node.ID())
		replica := inv.replicas[id]
		replica.node = node
	}

	inv.connected = true

	// this event is sent so that any delayed `replicaConnected` events can be processed.
	inv.eventLoop.AddEvent(ConnectedEvent{})

	return nil
}

// Propose sends the block to all replicas in the configuration
func (inv *Sender) Propose(proposal hotstuff.ProposeMsg) {
	cfg := inv.pbCfg
	ctx, cancel := timeout.Context(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()
	cfg.Propose(
		ctx,
		hotstuffpb.ProposalToProto(proposal),
	)
}

func (inv *Sender) replicaConnected(c hotstuff.ReplicaConnectedEvent) {
	info, peerOk := peer.FromContext(c.Ctx)
	md, mdOk := metadata.FromIncomingContext(c.Ctx)
	if !peerOk || !mdOk {
		return
	}

	id, err := inv.config.PeerIDFromContext(c.Ctx)
	if err != nil {
		inv.logger.Warnf("Failed to get id for %v: %v", info.Addr, err)
		return
	}

	_, ok := inv.config.ReplicaInfo(id)
	if !ok {
		inv.logger.Warnf("Replica with id %d was not found", id)
		return
	}

	replica := inv.replicas[id]
	replica.md = readMetadata(md)
	inv.config.SetReplicaMetaData(replica.id, replica.md)

	inv.logger.Debugf("Replica %d connected from address %v", id, info.Addr)
}

// Timeout sends the timeout message to all replicas.
func (inv *Sender) Timeout(msg hotstuff.TimeoutMsg) {
	cfg := inv.pbCfg

	// will wait until the second timeout before canceling
	ctx, cancel := timeout.Context(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()

	cfg.Timeout(
		ctx,
		hotstuffpb.TimeoutMsgToProto(msg),
	)
}

// Fetch requests a block from all the replicas in the configuration
func (inv *Sender) Fetch(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	cfg := inv.pbCfg

	protoBlock, err := cfg.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			inv.logger.Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

func (inv *Sender) ReplicaNode(id hotstuff.ID) (*Replica, bool) {
	rep, ok := inv.replicas[id]
	if !ok {
		return nil, false
	}
	return rep, ok
}

// Close closes all connections made by this configuration.
func (inv *Sender) Close() {
	inv.mgr.Close()
}

func (inv *Sender) GorumsConfig() gorums.RawConfiguration {
	return inv.pbCfg.RawConfiguration
}

// Sub returns a copy of self dedicated to the replica IDs provided.
func (inv *Sender) Sub(ids []hotstuff.ID) (*Sender, error) {
	replicas := make(map[hotstuff.ID]*Replica)
	nids := make([]uint32, len(ids))
	for i, id := range ids {
		nids[i] = uint32(id)
		replicas[id] = inv.replicas[id]
	}
	newCfg, err := inv.mgr.NewConfiguration(qspec{}, gorums.WithNodeIDs(nids))
	if err != nil {
		return nil, err
	}
	return &Sender{
		eventLoop: inv.eventLoop,
		logger:    inv.logger,
		config:    inv.config,
		pbCfg:     newCfg,
		replicas:  replicas,
	}, nil
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
