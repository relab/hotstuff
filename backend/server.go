package backend

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Server is the Server-side of the gorums backend.
// It is responsible for calling handler methods on the consensus instance.
type Server struct {
	blockChain    modules.BlockChain
	configuration modules.Configuration
	eventLoop     *eventloop.EventLoop
	logger        logging.Logger
	id            hotstuff.ID
	lm            latency.Matrix
	gorumsSrv     *gorums.Server
}

// InitModule initializes the Server.
func (srv *Server) InitModule(mods *modules.Core) {
	mods.Get(
		&srv.eventLoop,
		&srv.configuration,
		&srv.blockChain,
		&srv.logger,
	)
}

// NewServer creates a new Server.
func NewServer(opts ...ServerOptions) *Server {
	options := &backendOptions{}
	for _, opt := range opts {
		opt(options)
	}
	srv := &Server{
		id: options.id,
		lm: options.latencyMatrix,
	}
	options.gorumsSrvOpts = append(options.gorumsSrvOpts, gorums.WithConnectCallback(func(ctx context.Context) {
		srv.eventLoop.AddEvent(replicaConnected{ctx})
	}))
	srv.gorumsSrv = gorums.NewServer(options.gorumsSrvOpts...)
	hotstuffpb.RegisterHotstuffServer(srv.gorumsSrv, &serviceImpl{srv})
	return srv
}

// GetGorumsServer returns the underlying gorums Server.
func (srv *Server) GetGorumsServer() *gorums.Server {
	return srv.gorumsSrv
}

// Start creates a listener on the configured address and starts the server.
func (srv *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	srv.StartOnListener(lis)
	return nil
}

// StartOnListener starts the server with the given listener.
func (srv *Server) StartOnListener(listener net.Listener) {
	go func() {
		err := srv.gorumsSrv.Serve(listener)
		if err != nil {
			srv.logger.Errorf("An error occurred while serving: %v", err)
		}
	}()
}

// GetPeerIDFromContext extracts the ID of the peer from the context.
func GetPeerIDFromContext(ctx context.Context, cfg modules.Configuration) (hotstuff.ID, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("peerInfo not available")
	}

	if peerInfo.AuthInfo != nil && peerInfo.AuthInfo.AuthType() == "tls" {
		tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return 0, fmt.Errorf("authInfo of wrong type: %T", peerInfo.AuthInfo)
		}
		if len(tlsInfo.State.PeerCertificates) > 0 {
			cert := tlsInfo.State.PeerCertificates[0]
			for replicaID := range cfg.Replicas() {
				if subject, err := strconv.Atoi(cert.Subject.CommonName); err == nil && hotstuff.ID(subject) == replicaID {
					return replicaID, nil
				}
			}
		}
		return 0, fmt.Errorf("could not find matching certificate")
	}

	// If we're not using TLS, we'll fallback to checking the metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, fmt.Errorf("metadata not available")
	}

	v := md.Get("id")
	if len(v) < 1 {
		return 0, fmt.Errorf("id field not present")
	}

	id, err := strconv.Atoi(v[0])
	if err != nil {
		return 0, fmt.Errorf("cannot parse ID field: %w", err)
	}

	return hotstuff.ID(id), nil
}

// Stop stops the server.
func (srv *Server) Stop() {
	srv.gorumsSrv.Stop()
}

// serviceImpl provides the implementation of the HotStuff gorums service.
type serviceImpl struct {
	srv *Server
}

func (impl *serviceImpl) addMessageEvent(event any, to hotstuff.ID) {
	if !impl.srv.lm.Enabled() {
		impl.srv.eventLoop.AddEvent(event)
		return
	}

	delay := impl.srv.lm.Latency(impl.srv.id, to)
	impl.srv.logger.Debugf("Delay between %s and %s: %v\n", impl.srv.lm.Location(impl.srv.id), impl.srv.lm.Location(to), delay)
	impl.srv.eventLoop.DelayEvent(event, delay)
}

// Propose handles a replica's response to the Propose QC from the leader.
func (impl *serviceImpl) Propose(ctx gorums.ServerCtx, proposal *hotstuffpb.Proposal) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}
	proposal.Block.Proposer = uint32(id)
	proposeMsg := hotstuffpb.ProposalFromProto(proposal)
	proposeMsg.ID = id

	impl.addMessageEvent(proposeMsg, id)
}

// Vote handles an incoming vote message.
func (impl *serviceImpl) Vote(ctx gorums.ServerCtx, cert *hotstuffpb.PartialCert) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}

	voteMsg := hotstuff.VoteMsg{
		ID:          id,
		PartialCert: hotstuffpb.PartialCertFromProto(cert),
	}

	impl.addMessageEvent(voteMsg, id)
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (impl *serviceImpl) NewView(ctx gorums.ServerCtx, msg *hotstuffpb.SyncInfo) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}

	newViewMsg := hotstuff.NewViewMsg{
		ID:       id,
		SyncInfo: hotstuffpb.SyncInfoFromProto(msg),
	}

	impl.addMessageEvent(newViewMsg, id)
}

// Fetch handles an incoming fetch request.
func (impl *serviceImpl) Fetch(_ gorums.ServerCtx, pb *hotstuffpb.BlockHash) (*hotstuffpb.Block, error) {
	var hash hotstuff.Hash
	copy(hash[:], pb.GetHash())

	block, ok := impl.srv.blockChain.LocalGet(hash)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "requested block was not found")
	}

	impl.srv.logger.Debugf("OnFetch: %.8s", hash)

	return hotstuffpb.BlockToProto(block), nil
}

// Timeout handles an incoming TimeoutMsg.
func (impl *serviceImpl) Timeout(ctx gorums.ServerCtx, msg *hotstuffpb.TimeoutMsg) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
	}
	timeoutMsg := hotstuffpb.TimeoutMsgFromProto(msg)
	timeoutMsg.ID = id

	impl.addMessageEvent(timeoutMsg, id)
}

type replicaConnected struct {
	ctx context.Context
}
