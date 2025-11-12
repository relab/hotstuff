// Package network provides an implementation of a Gorums-based sender for HotStuff messages.
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type GorumsSender struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	mgrOpts   []gorums.ManagerOption
	connected bool

	mgr      *hotstuffpb.Manager
	replicas map[hotstuff.ID]*replicaNode

	pbCfg *hotstuffpb.Configuration
}

func NewGorumsSender(
	el *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *GorumsSender {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}
	mgrOpts = append(mgrOpts, gorums.WithGrpcDialOptions(grpcOpts...))

	s := &GorumsSender{
		eventLoop: el,
		logger:    logger,
		config:    config,

		mgrOpts:  mgrOpts,
		replicas: make(map[hotstuff.ID]*replicaNode),
	}

	// We delay processing `replicaConnected` events until after the configurations `connected` event has occurred.
	eventloop.Register(el, func(event hotstuff.ReplicaConnectedEvent) {
		if !s.connected {
			eventloop.DelayUntil[ConnectedEvent](el, event)
			return
		}
		s.replicaConnected(event)
	})
	return s
}

func (s *GorumsSender) Connect(replicas []hotstuff.ReplicaInfo) (err error) {
	mgrOpts := s.mgrOpts
	md := mapToMetadata(s.config.ConnectionMetadata())
	// embed own ID to allow other replicas to identify messages from this replica
	md.Set("id", fmt.Sprintf("%d", s.config.ID()))
	mgrOpts = append(mgrOpts, gorums.WithMetadata(md))
	s.mgr = hotstuffpb.NewManager(mgrOpts...)
	// set up an ID mapping to give to gorums
	idMapping := make(map[string]uint32, len(replicas))
	for _, replica := range replicas {
		// also initialize Replica structures
		realReplica := &replicaNode{
			eventLoop: s.eventLoop,
			id:        replica.ID,
			pubKey:    replica.PubKey,
			// node and metaData is set later
		}
		s.replicas[replica.ID] = realReplica
		// add the info to the config
		s.config.AddReplica(&replica)
		// we do not want to connect to ourself
		if replica.ID != s.config.ID() {
			idMapping[replica.Address] = uint32(replica.ID)
		}
	}
	// this will connect to the replicas
	s.pbCfg, err = s.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(idMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}
	// now we need to update the "node" field of each replica we connected to
	for _, node := range s.pbCfg.Nodes() {
		// the node ID should correspond with the replica ID
		// because we already configured an ID mapping for gorums to use.
		id := hotstuff.ID(node.ID())
		replica := s.replicas[id]
		replica.node = node
	}
	s.connected = true
	// this event is sent so that any delayed `replicaConnected` events can be processed.
	s.eventLoop.AddEvent(ConnectedEvent{})
	return nil
}

// Propose sends the block to all replicas in the configuration
func (s *GorumsSender) Propose(proposal *hotstuff.ProposeMsg) {
	cfg := s.pbCfg
	ctx, cancel := s.eventLoop.TimeoutContext()
	defer cancel()
	cfg.Propose(
		ctx,
		hotstuffpb.ProposalToProto(*proposal),
	)
}

func (s *GorumsSender) replicaConnected(c hotstuff.ReplicaConnectedEvent) {
	info, peerOk := peer.FromContext(c.Ctx)
	md, mdOk := metadata.FromIncomingContext(c.Ctx)
	if !peerOk || !mdOk {
		return
	}

	id, err := s.config.PeerIDFromContext(c.Ctx)
	if err != nil {
		s.logger.Warnf("Failed to get id for %v: %v", info.Addr, err)
		return
	}

	_, ok := s.config.ReplicaInfo(id)
	if !ok {
		s.logger.Warnf("Replica with id %d was not found", id)
		return
	}

	replica := s.replicas[id]
	replica.md = readMetadata(md)
	if err := s.config.SetReplicaMetadata(replica.id, replica.md); err != nil {
		s.logger.Errorf("failed to set replica metadata: %v", err)
		return
	}

	s.logger.Debugf("Replica %d connected from address %v", id, info.Addr)
}

// Timeout sends the timeout message to all replicas.
func (s *GorumsSender) Timeout(msg hotstuff.TimeoutMsg) {
	cfg := s.pbCfg

	// will wait until the second timeout before canceling
	ctx, cancel := s.eventLoop.TimeoutContext()
	defer cancel()

	cfg.Timeout(
		ctx,
		hotstuffpb.TimeoutMsgToProto(msg),
	)
}

// RequestBlock requests a block from all the replicas in the configuration
func (s *GorumsSender) RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	cfg := s.pbCfg
	protoBlock, err := cfg.RequestBlock(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			s.logger.Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

func (s *GorumsSender) ReplicaExists(id hotstuff.ID) bool {
	_, ok := s.replicas[id]
	return ok
}

// NewView sends the quorum certificate to the other replica.
func (s *GorumsSender) NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error {
	r, ok := s.replicas[id]
	if !ok {
		return fmt.Errorf("replica does not exist (id=%d)", id)
	}
	r.newView(msg)
	return nil
}

// Vote sends the partial certificate to the other replica.
func (s *GorumsSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	r, ok := s.replicas[id]
	if !ok {
		return fmt.Errorf("replica does not exist (id=%d)", id)
	}
	r.vote(cert)
	return nil
}

// Close closes all connections made by this configuration.
func (s *GorumsSender) Close() {
	s.mgr.Close()
}

func (s *GorumsSender) GorumsConfig() gorums.RawConfiguration {
	return s.pbCfg.RawConfiguration
}

// Sub returns a copy of self dedicated to the replica IDs provided.
func (s *GorumsSender) Sub(ids []hotstuff.ID) (core.Sender, error) {
	replicas := make(map[hotstuff.ID]*replicaNode)
	nids := make([]uint32, len(ids))
	for i, id := range ids {
		nids[i] = uint32(id)
		replicas[id] = s.replicas[id]
	}
	newCfg, err := s.mgr.NewConfiguration(qspec{}, gorums.WithNodeIDs(nids))
	if err != nil {
		return nil, err
	}
	return &GorumsSender{
		eventLoop: s.eventLoop,
		logger:    s.logger,
		config:    s.config,
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

// RequestBlockQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) RequestBlockQF(in *hotstuffpb.BlockHash, replies map[uint32]*hotstuffpb.Block) (*hotstuffpb.Block, bool) {
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

var _ core.Sender = (*GorumsSender)(nil)
