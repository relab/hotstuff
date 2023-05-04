package backend

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
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
	location      string
	locationInfo  map[hotstuff.ID]string
	latencyMatrix map[string]time.Duration
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
		location:      options.location,
		locationInfo:  options.locationInfo,
		latencyMatrix: options.locationLatencies,
	}
	options.gorumsSrvOpts = append(options.gorumsSrvOpts, gorums.WithConnectCallback(func(ctx context.Context) {
		srv.eventLoop.AddEvent(replicaConnected{ctx})
	}))
	srv.gorumsSrv = gorums.NewServer(options.gorumsSrvOpts...)
	hotstuffpb.RegisterHotstuffServer(srv.gorumsSrv, &serviceImpl{srv})
	return srv
}

func (srv *Server) induceLatency(sender hotstuff.ID) {
	if srv.location == hotstuff.DefaultLocation {
		return
	}
	senderLocation := srv.locationInfo[sender]
	senderLatency := srv.latencyMatrix[senderLocation]
	srv.logger.Debugf("latency from server %s to server %s is %s\n", srv.location, senderLocation, senderLatency)
	timer1 := time.NewTimer(senderLatency)
	<-timer1.C
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

// Propose handles a replica's response to the Propose QC from the leader.
func (impl *serviceImpl) Propose(ctx gorums.ServerCtx, proposal *hotstuffpb.Proposal) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Infof("Failed to get client ID: %v", err)
		return
	}

	proposal.Block.Proposer = uint32(id)
	proposeMsg := hotstuffpb.ProposalFromProto(proposal)
	proposeMsg.ID = id
	impl.srv.induceLatency(id)
	impl.srv.eventLoop.AddEvent(proposeMsg)
}

// Vote handles an incoming vote message.
func (impl *serviceImpl) Vote(ctx gorums.ServerCtx, cert *hotstuffpb.PartialCert) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Infof("Failed to get client ID: %v", err)
		return
	}
	impl.srv.induceLatency(id)
	impl.srv.eventLoop.AddEvent(hotstuff.VoteMsg{
		ID:          id,
		PartialCert: hotstuffpb.PartialCertFromProto(cert),
	})
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (impl *serviceImpl) NewView(ctx gorums.ServerCtx, msg *hotstuffpb.SyncInfo) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Infof("Failed to get client ID: %v", err)
		return
	}
	impl.srv.induceLatency(id)
	impl.srv.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       id,
		SyncInfo: hotstuffpb.SyncInfoFromProto(msg),
	})
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
	var err error
	timeoutMsg := hotstuffpb.TimeoutMsgFromProto(msg)
	timeoutMsg.ID, err = GetPeerIDFromContext(ctx, impl.srv.configuration)
	if err != nil {
		impl.srv.logger.Infof("Could not get ID of replica: %v", err)
	}
	impl.srv.induceLatency(timeoutMsg.ID)
	impl.srv.eventLoop.AddEvent(timeoutMsg)
}

type replicaConnected struct {
	ctx context.Context
}
