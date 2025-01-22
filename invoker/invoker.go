package invoker

import (
	"context"
	"fmt"
	"strings"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/convert"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/synctools"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type Invoker struct {
	configuration *netconfig.Config
	eventLoop     *core.EventLoop
	logger        logging.Logger
	opts          *core.Options

	mgrOpts   []gorums.ManagerOption
	connected bool

	mgr      *hotstuffpb.Manager
	replicas map[hotstuff.ID]*Replica

	pbCfg *hotstuffpb.Configuration
}

func New(creds credentials.TransportCredentials, opts ...gorums.ManagerOption) *Invoker {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	// initialization will be finished by InitModule
	return &Invoker{
		mgrOpts:  opts,
		replicas: make(map[hotstuff.ID]*Replica),
	}
}

func (inv *Invoker) InitModule(mods *core.Core) {
	mods.Get(
		&inv.configuration,
		&inv.eventLoop,
		&inv.logger,
		&inv.opts,
	)

	// We delay processing `replicaConnected` events until after the configurations `connected` event has occurred.
	inv.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(event any) {
		if !inv.connected {
			inv.eventLoop.DelayUntil(ConnectedEvent{}, event)
			return
		}
		inv.replicaConnected(event.(hotstuff.ReplicaConnectedEvent))
	})
}

func (inv *Invoker) Connect(replicas []hotstuff.ReplicaInfo) (err error) {
	mgrOpts := inv.mgrOpts
	// TODO(AlanRostem): this was here when subConfig existed. Check if doing this is valid
	// cfg.opts = nil // options are not needed beyond this point, so we delete them.

	md := mapToMetadata(inv.opts.ConnectionMetadata())

	// embed own ID to allow other replicas to identify messages from this replica
	md.Set("id", fmt.Sprintf("%d", inv.opts.ID()))

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
		inv.configuration.AddReplica(replica)
		// we do not want to connect to ourself
		if replica.ID != inv.opts.ID() {
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
func (inv *Invoker) Propose(proposal hotstuff.ProposeMsg) {
	cfg := inv.pbCfg
	ctx, cancel := synctools.TimeoutContext(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()
	cfg.Propose(
		ctx,
		convert.ProposalToProto(proposal),
	)
}

func (inv *Invoker) replicaConnected(c hotstuff.ReplicaConnectedEvent) {
	info, peerok := peer.FromContext(c.Ctx)
	md, mdok := metadata.FromIncomingContext(c.Ctx)
	if !peerok || !mdok {
		return
	}

	id, err := inv.configuration.PeerIDFromContext(c.Ctx)
	if err != nil {
		inv.logger.Warnf("Failed to get id for %v: %v", info.Addr, err)
		return
	}

	_, ok := inv.configuration.Replica(id)
	if !ok {
		inv.logger.Warnf("Replica with id %d was not found", id)
		return
	}

	replica := inv.replicas[id]
	replica.md = readMetadata(md)

	inv.logger.Debugf("Replica %d connected from address %v", id, info.Addr)
}

// Timeout sends the timeout message to all replicas.
func (inv *Invoker) Timeout(msg hotstuff.TimeoutMsg) {
	cfg := inv.pbCfg

	// will wait until the second timeout before canceling
	ctx, cancel := synctools.TimeoutContext(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()

	cfg.Timeout(
		ctx,
		convert.TimeoutMsgToProto(msg),
	)
}

// Fetch requests a block from all the replicas in the configuration
func (inv *Invoker) Fetch(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
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
	return convert.BlockFromProto(protoBlock), true
}

func (inv *Invoker) ReplicaNode(id hotstuff.ID) (*Replica, bool) {
	rep, ok := inv.replicas[id]
	if !ok {
		return nil, false
	}
	return rep, ok
}

// Close closes all connections made by this configuration.
func (inv *Invoker) Close() {
	inv.mgr.Close()
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
		block := convert.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}

// ConnectedEvent is sent when the configuration has connected to the other replicas.
type ConnectedEvent struct{}
