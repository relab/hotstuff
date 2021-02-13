package gorums

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

var logger = logging.GetLogger()

// Server is the server-side of the gorums backend.
// It is responsible for calling handler methods on the consensus instance.
type Server struct {
	addr      string
	hs        hotstuff.Consensus
	gorumsSrv *gorums.Server
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
func (srv *Server) Start(hs hotstuff.Consensus) error {
	lis, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", srv.addr, err)
	}
	srv.StartOnListener(hs, lis)
	return nil
}

// StartOnListener starts the server on the given listener.
func (srv *Server) StartOnListener(hs hotstuff.Consensus, listener net.Listener) {
	srv.hs = hs
	go func() {
		err := srv.gorumsSrv.Serve(listener)
		if err != nil {
			logger.Errorf("An error occurred while serving: %v", err)
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
			for replicaID := range srv.hs.Config().Replicas() {
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
func (srv *Server) Propose(ctx context.Context, block *proto.Block) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		logger.Infof("Failed to get client ID: %v", err)
		return
	}
	// defaults to 0 if error
	block.Proposer = uint32(id)
	srv.hs.OnPropose(proto.BlockFromProto(block))
}

// Vote handles an incoming vote message.
func (srv *Server) Vote(_ context.Context, cert *proto.PartialCert) {
	srv.hs.OnVote(proto.PartialCertFromProto(cert))
}

// NewView handles the leader's response to receiving a NewView rpc from a replica.
func (srv *Server) NewView(ctx context.Context, msg *proto.NewViewMsg) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		logger.Infof("Failed to get client ID: %v", err)
		return
	}
	srv.hs.OnNewView(hotstuff.NewView{
		ID:   id,
		View: hotstuff.View(msg.GetView()),
		QC:   proto.QuorumCertFromProto(msg.GetQC()),
	})
}

// Fetch handles an incoming fetch request.
func (srv *Server) Fetch(ctx context.Context, pb *proto.BlockHash) {
	var hash hotstuff.Hash
	copy(hash[:], pb.GetHash())

	block, ok := srv.hs.BlockChain().Get(hash)
	if !ok {
		return
	}

	logger.Debugf("OnFetch: %.8s", hash)

	id, err := srv.getClientID(ctx)
	if err != nil {
		logger.Infof("Fetch: could not get peer id: %v", err)
	}

	replica, ok := srv.hs.Config().Replica(id)
	if !ok {
		logger.Infof("Fetch: could not find replica with id: %d", id)
		return
	}

	replica.Deliver(block)
}

// Deliver handles an incoming deliver message.
func (srv *Server) Deliver(_ context.Context, block *proto.Block) {
	srv.hs.OnDeliver(proto.BlockFromProto(block))
}
