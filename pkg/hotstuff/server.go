package hotstuff

import (
	"context"

	"github.com/relab/hotstuff/pkg/proto"
)

// a simple struct implementing the hotstuff server API which will call back to replica methods
type hotstuffServer struct {
	hs *HotStuff
}

// Propose handles a replica's response to the Propose QC from the leader
func (s *hotstuffServer) Propose(ctx context.Context, node *proto.HSNode) (*proto.PartialCert, error) {
	n := nodeFromProto(node)
	p, err := s.hs.onReceiveProposal(n)
	if err != nil {
		return nil, err
	}
	return p.toProto(), nil
}

// NewView handles the leader's response to receiving a NewView rpc from a replica
func (s *hotstuffServer) NewView(ctx context.Context, msg *proto.QuorumCert) (*proto.Empty, error) {
	qc := quorumCertFromProto(msg)
	s.hs.onReceiveNewView(qc)
	return &proto.Empty{}, nil
}

// LeaderChange handles an incoming LeaderUpdate message for a new leader.
func (s *hotstuffServer) LeaderChange(ctx context.Context, msg *proto.LeaderUpdate) (*proto.Empty, error) {
	qc := quorumCertFromProto(msg.GetQC())
	sig := partialSigFromProto(msg.GetSig())
	s.hs.onReceiveLeaderChange(qc, sig)
	return &proto.Empty{}, nil
}
