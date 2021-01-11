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

type Server struct {
	addr      string
	hs        hotstuff.Consensus
	gorumsSrv *gorums.Server
}

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

func (srv *Server) StartServer(hs hotstuff.Consensus) error {
	srv.hs = hs
	lis, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return fmt.Errorf("Failed to listen on %s: %w", srv.addr, err)
	}
	go srv.gorumsSrv.Serve(lis)
	return nil
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

// Propose handles a replica's response to the Propose QC from the leader
func (srv *Server) Propose(ctx context.Context, block *proto.Block) {
	id, err := srv.getClientID(ctx)
	if err != nil {
		logger.Infof("Failed to get client ID: %v", err)
		return
	}
	// defaults to 0 if error
	block.XProposer = uint32(id)
	srv.hs.OnPropose(block)
}

func (srv *Server) Vote(ctx context.Context, cert *proto.PartialCert) {
	srv.hs.OnVote(cert)
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (srv *Server) NewView(ctx context.Context, msg *proto.QuorumCert) {
	srv.hs.OnNewView(msg)
}
