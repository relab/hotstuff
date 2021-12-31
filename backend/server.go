package backend

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Server is the Server-side of the gorums backend.
// It is responsible for calling handler methods on the consensus instance.
type Server struct {
	mods      *consensus.Modules
	gorumsSrv *gorums.Server
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (srv *Server) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	srv.mods = mods
}

// NewServer creates a new Server.
func NewServer(opts ...gorums.ServerOption) *Server {
	srv := &Server{}

	grpcServerOpts := []grpc.ServerOption{}

	opts = append(opts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	srv.gorumsSrv = gorums.NewServer(opts...)

	hotstuffpb.RegisterHotstuffServer(srv.gorumsSrv, &serviceImpl{srv})
	return srv
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
			srv.mods.Logger().Errorf("An error occurred while serving: %v", err)
		}
	}()
}

// GetPeerIDFromContext extracts the ID of the peer from the context.
func GetPeerIDFromContext(ctx context.Context, cfg consensus.Configuration) (hotstuff.ID, error) {
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
	id, err := GetPeerIDFromContext(ctx, impl.srv.mods.Configuration())
	if err != nil {
		impl.srv.mods.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	proposal.Block.Proposer = uint32(id)
	proposeMsg := hotstuffpb.ProposalFromProto(proposal)
	proposeMsg.ID = id

	impl.srv.mods.EventLoop().AddEvent(proposeMsg)
}

// Vote handles an incoming vote message.
func (impl *serviceImpl) Vote(ctx gorums.ServerCtx, cert *hotstuffpb.PartialCert) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.mods.Configuration())
	if err != nil {
		impl.srv.mods.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	impl.srv.mods.EventLoop().AddEvent(consensus.VoteMsg{
		ID:          id,
		PartialCert: hotstuffpb.PartialCertFromProto(cert),
	})
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (impl *serviceImpl) NewView(ctx gorums.ServerCtx, msg *hotstuffpb.SyncInfo) {
	id, err := GetPeerIDFromContext(ctx, impl.srv.mods.Configuration())
	if err != nil {
		impl.srv.mods.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	impl.srv.mods.EventLoop().AddEvent(consensus.NewViewMsg{
		ID:       id,
		SyncInfo: hotstuffpb.SyncInfoFromProto(msg),
	})
}

// Fetch handles an incoming fetch request.
func (impl *serviceImpl) Fetch(ctx gorums.ServerCtx, pb *hotstuffpb.BlockHash) (*hotstuffpb.Block, error) {
	var hash consensus.Hash
	copy(hash[:], pb.GetHash())

	block, ok := impl.srv.mods.BlockChain().LocalGet(hash)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "requested block was not found")
	}

	impl.srv.mods.Logger().Debugf("OnFetch: %.8s", hash)

	return hotstuffpb.BlockToProto(block), nil
}

// Timeout handles an incoming TimeoutMsg.
func (impl *serviceImpl) Timeout(ctx gorums.ServerCtx, msg *hotstuffpb.TimeoutMsg) {
	var err error
	timeoutMsg := hotstuffpb.TimeoutMsgFromProto(msg)
	timeoutMsg.ID, err = GetPeerIDFromContext(ctx, impl.srv.mods.Configuration())
	if err != nil {
		impl.srv.mods.Logger().Infof("Could not get ID of replica: %v", err)
	}
	impl.srv.mods.EventLoop().AddEvent(timeoutMsg)
}
