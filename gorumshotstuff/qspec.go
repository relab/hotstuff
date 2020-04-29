package gorumshotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
)

type hotstuffQSpec struct {
	*hotstuff.ReplicaConfig
	verified map[*proto.PartialCert]bool
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (spec hotstuffQSpec) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.QuorumCert, bool) {
	if spec.verified == nil {
		spec.verified = make(map[*proto.PartialCert]bool)
	}

	// -1 because we self voted earlier
	if len(replies) < spec.QuorumSize-1 {
		return nil, false
	}

	numVerified := 0
	for _, pc := range replies {
		if ok, verifiedBefore := spec.verified[pc]; !verifiedBefore {
			cert := pc.FromProto()
			ok := hotstuff.VerifyPartialCert(spec.ReplicaConfig, cert)
			if ok {
				numVerified++
			}
			spec.verified[pc] = ok
		} else if ok {
			numVerified++
		}
	}

	if numVerified >= spec.QuorumSize-1 {
		qc := hotstuff.CreateQuorumCert(req.FromProto())
		for c, ok := range spec.verified {
			if ok {
				qc.AddPartial(c.FromProto())
			}
		}
		spec.verified = nil
		return proto.QuorumCertToProto(qc), true
	}

	return nil, false
}
