package hotstuff

import (
	"context"
	"fmt"
	"net"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/hotstuff/pkg/proto"
	"google.golang.org/grpc"
)

// a simple struct implementing the hotstuff server API which will call back to replica methods
type hotstuffServer struct {
	hs *HotStuff
}

// Propose handles a replica's response to the Propose QC from the leader
func (s *hotstuffServer) Propose(ctx context.Context, node *proto.HSNode) (*proto.PartialCert, error) {
	n := nodeFromProto(node)
	p, err := s.hs.onReceiveProposal(n)
	return p.toProto(), err
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (s *hotstuffServer) NewView(ctx context.Context, msg *proto.QuorumCert) (*empty.Empty, error) {
	qc := quorumCertFromProto(msg)
	s.hs.onReceiveNewView(qc)
	return &empty.Empty{}, nil
}

// LeaderChange handles an incoming LeaderUpdate message for a new leader.
func (s *hotstuffServer) LeaderChange(ctx context.Context, msg *proto.LeaderUpdate) (*empty.Empty, error) {
	qc := quorumCertFromProto(msg.GetQC())
	sig := partialSigFromProto(msg.GetSig())
	s.hs.onReceiveLeaderChange(qc, sig)
	return &empty.Empty{}, nil
}

// StartServer runs a new instance of hotstuffServer
func StartServer(replica *HotStuff, port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("Failed to listen to port %s: %w", port, err)
	}
	grpcServer := grpc.NewServer()
	hs := &hotstuffServer{replica}
	proto.RegisterHotstuffServer(grpcServer, hs)
	go grpcServer.Serve(lis)
	return nil
}
