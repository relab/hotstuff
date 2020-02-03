package hotstuff

import (
	"github.com/relab/hotstuff/pkg/proto"
)

type hotstuffQSpec struct {
	*ReplicaConfig
	QC *QuorumCert
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (spec hotstuffQSpec) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.Empty, bool) {
	logger.Printf("ProposeQF: %d replies\n", len(replies))
	// -1 because we self voted earlier
	if len(replies) < spec.QuorumSize-1 {
		return &proto.Empty{}, false
	}

	for _, pc := range replies {
		// AddPartial does deduplication, but not verification
		err := spec.QC.AddPartial(partialCertFromProto(pc))
		if err != nil {
			logger.Println("Could not add partial cert to QC: ", err)
		}
	}

	// TODO: find a way to avoid checking a signature more than once
	if VerifyQuorumCert(spec.ReplicaConfig, spec.QC) {
		return &proto.Empty{}, true
	}

	return &proto.Empty{}, false
}
