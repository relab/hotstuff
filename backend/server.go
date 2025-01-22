package backend

import (
	"context"
	"fmt"
	"net"

	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/convert"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/netconfig"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server is the Server-side of the gorums backend.
// It is responsible for calling handler methods on the consensus instance.
type Server struct {
	blockChain    *blockchain.BlockChain
	configuration *netconfig.Config
	eventLoop     *core.EventLoop
	logger        logging.Logger
	id            hotstuff.ID
	lm            latency.Matrix
	gorumsSrv     *gorums.Server
}

// InitModule initializes the Server.
func (srv *Server) InitModule(mods *core.Core) {
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
		srv.eventLoop.AddEvent(hotstuff.ReplicaConnectedEvent{Ctx: ctx})
	}))
	srv.gorumsSrv = gorums.NewServer(options.gorumsSrvOpts...)
	hotstuffpb.RegisterHotstuffServer(srv.gorumsSrv, &serviceImpl{srv})
	return srv
}

// addNetworkDelay adds latency between this server and the sender based on
// the latency between the two locations according to the latency matrix.
func (srv *Server) addNetworkDelay(sender hotstuff.ID) {
	if !srv.lm.Enabled() {
		return
	}
	delay := srv.lm.Latency(srv.id, sender)
	srv.logger.Debugf("Delay between %s and %s: %v\n", srv.lm.Location(srv.id), srv.lm.Location(sender), delay)
	srv.lm.Delay(srv.id, sender)
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
	id, err := impl.srv.configuration.PeerIDFromContext(ctx)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}
	proposal.Block.Proposer = uint32(id)
	proposeMsg := convert.ProposalFromProto(proposal)
	proposeMsg.ID = id
	impl.srv.addNetworkDelay(id)
	impl.srv.eventLoop.AddEvent(proposeMsg)
}

// Vote handles an incoming vote message.
func (impl *serviceImpl) Vote(ctx gorums.ServerCtx, cert *hotstuffpb.PartialCert) {
	id, err := impl.srv.configuration.PeerIDFromContext(ctx)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}
	impl.srv.addNetworkDelay(id)
	impl.srv.eventLoop.AddEvent(hotstuff.VoteMsg{
		ID:          id,
		PartialCert: convert.PartialCertFromProto(cert),
	})
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (impl *serviceImpl) NewView(ctx gorums.ServerCtx, msg *hotstuffpb.SyncInfo) {
	id, err := impl.srv.configuration.PeerIDFromContext(ctx)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
		return
	}
	impl.srv.addNetworkDelay(id)
	impl.srv.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       id,
		SyncInfo: convert.SyncInfoFromProto(msg),
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

	return convert.BlockToProto(block), nil
}

// Timeout handles an incoming TimeoutMsg.
func (impl *serviceImpl) Timeout(ctx gorums.ServerCtx, msg *hotstuffpb.TimeoutMsg) {
	id, err := impl.srv.configuration.PeerIDFromContext(ctx)
	if err != nil {
		impl.srv.logger.Warnf("Could not get replica ID: %v", err)
	}
	timeoutMsg := convert.TimeoutMsgFromProto(msg)
	timeoutMsg.ID = id
	impl.srv.addNetworkDelay(id)
	impl.srv.eventLoop.AddEvent(timeoutMsg)
}
