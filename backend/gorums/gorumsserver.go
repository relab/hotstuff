package gorums

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Server is the server-side of the gorums backend.
// It is responsible for calling handler methods on the consensus instance.
type Server struct {
	mod       *hotstuff.HotStuff
	addr      string
	gorumsSrv *gorums.Server
}

// InitModule initializes the server with the given HotStuff instance.
func (srv *Server) InitModule(hs *hotstuff.HotStuff, _ *hotstuff.OptionsBuilder) {
	srv.mod = hs
}

// NewServer creates a new Server.
func NewServer(replicaConfig config.ReplicaConfig) *Server {
	srv := &Server{}
	srv.addr = replicaConfig.Replicas[replicaConfig.ID].Address

	serverOpts := []gorums.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if replicaConfig.Creds != nil {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(replicaConfig.Creds.Clone()))
	}

	serverOpts = append(serverOpts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	srv.gorumsSrv = gorums.NewServer(serverOpts...)

	proto.RegisterHotstuffServer(srv.gorumsSrv, srv)
	return srv
}

// Start creates a listener on the configured address and starts the server.
func (srv *Server) Start() error {
	lis, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", srv.addr, err)
	}
	srv.StartOnListener(lis)
	return nil
}

// StartOnListener starts the server on the given listener.
func (srv *Server) StartOnListener(listener net.Listener) {
	go func() {
		err := srv.gorumsSrv.Serve(listener)
		if err != nil {
			srv.mod.Logger().Errorf("An error occurred while serving: %v", err)
		}
	}()
}

func (srv *Server) getClientID(ctx context.Context) (hotstuff.ID, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("getClientID: peerInfo not available")
	}

	if peerInfo.AuthInfo != nil && peerInfo.AuthInfo.AuthType() == "tls" {
		tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return 0, fmt.Errorf("getClientID: authInfo of wrong type: %T", peerInfo.AuthInfo)
		}
		if len(tlsInfo.State.PeerCertificates) > 0 {
			cert := tlsInfo.State.PeerCertificates[0]
			for replicaID := range srv.mod.Config().Replicas() {
				if subject, err := strconv.Atoi(cert.Subject.CommonName); err == nil && hotstuff.ID(subject) == replicaID {
					return replicaID, nil
				}
			}
		}
		return 0, fmt.Errorf("getClientID: could not find matching certificate")
	}

	// If we're not using TLS, we'll fallback to checking the metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, fmt.Errorf("getClientID: metadata not available")
	}

	v := md.Get("id")
	if len(v) < 1 {
		return 0, fmt.Errorf("getClientID: id field not present")
	}

	id, err := strconv.Atoi(v[0])
	if err != nil {
		return 0, fmt.Errorf("getClientID: cannot parse ID field: %w", err)
	}

	return hotstuff.ID(id), nil
}

// Stop stops the server.
func (srv *Server) Stop() {
	srv.gorumsSrv.Stop()
}

// Propose handles a replica's response to the Propose QC from the leader.
func (srv *Server) Propose(ctx context.Context, proposal *proto.Proposal) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		srv.mod.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	proposal.Block.Proposer = uint32(id)
	proposeMsg := proto.ProposalFromProto(proposal)
	proposeMsg.ID = id

	srv.mod.EventLoop().AddEvent(proposeMsg)
}

// Vote handles an incoming vote message.
func (srv *Server) Vote(ctx context.Context, cert *proto.PartialCert) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		srv.mod.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	srv.mod.EventLoop().AddEvent(hotstuff.VoteMsg{
		ID:          id,
		PartialCert: proto.PartialCertFromProto(cert),
	})
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (srv *Server) NewView(ctx context.Context, msg *proto.SyncInfo) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		srv.mod.Logger().Infof("Failed to get client ID: %v", err)
		return
	}

	srv.mod.EventLoop().AddEvent(hotstuff.NewViewMsg{
		ID:       id,
		SyncInfo: proto.SyncInfoFromProto(msg),
	})
}

// Fetch handles an incoming fetch request.
func (srv *Server) Fetch(ctx context.Context, pb *proto.BlockHash, respond func(block *proto.Block, err error)) {
	var hash hotstuff.Hash
	copy(hash[:], pb.GetHash())

	block, ok := srv.mod.BlockChain().LocalGet(hash)
	if !ok {
		respond(nil, status.Errorf(codes.NotFound, "requested block was not found"))
		return
	}

	srv.mod.Logger().Debugf("OnFetch: %.8s", hash)

	respond(proto.BlockToProto(block), nil)
}

// Timeout handles an incoming TimeoutMsg.
func (srv *Server) Timeout(ctx context.Context, msg *proto.TimeoutMsg) {
	var err error
	timeoutMsg := proto.TimeoutMsgFromProto(msg)
	timeoutMsg.ID, err = srv.getClientID(ctx)
	if err != nil {
		srv.mod.Logger().Infof("Could not get ID of replica: %v", err)
	}
	srv.mod.EventLoop().AddEvent(timeoutMsg)
}
