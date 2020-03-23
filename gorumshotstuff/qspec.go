package gorumshotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
)

type hotstuffQSpec struct {
	*hotstuff.ReplicaConfig
	QC *hotstuff.QuorumCert
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (spec hotstuffQSpec) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.Empty, bool) {
	// -1 because we self voted earlier
	if len(replies) < spec.QuorumSize-1 {
		return &proto.Empty{}, false
	}

	for _, pc := range replies {
		// AddPartial does deduplication, but not verification
		spec.QC.AddPartial(pc.FromProto())
	}

	// TODO: find a way to avoid checking a signature more than once
	if hotstuff.VerifyQuorumCert(spec.ReplicaConfig, spec.QC) {
		return &proto.Empty{}, true
	}

	return &proto.Empty{}, false
}
