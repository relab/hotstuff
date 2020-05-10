package gorumshotstuff

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
)

type hotstuffQSpec struct {
	*hotstuff.ReplicaConfig
	verified map[*proto.PartialCert]bool
	jobs     chan struct {
		pc *proto.PartialCert
		ok bool
	}
	wg sync.WaitGroup
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (spec *hotstuffQSpec) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.QuorumCert, bool) {
	if spec.verified == nil {
		spec.verified = make(map[*proto.PartialCert]bool)
	}
	if spec.jobs == nil {
		spec.jobs = make(chan struct {
			pc *proto.PartialCert
			ok bool
		}, len(spec.Replicas))
	}

	numVerified := 0
	for _, pc := range replies {
		if ok, verifiedBefore := spec.verified[pc]; !verifiedBefore {
			spec.verified[pc] = false
			spec.wg.Add(1)
			go func(pc *proto.PartialCert) {
				cert := pc.FromProto()
				ok := hotstuff.VerifyPartialCert(spec.ReplicaConfig, cert)
				spec.jobs <- struct {
					pc *proto.PartialCert
					ok bool
				}{pc, ok}
				spec.wg.Done()
			}(pc)
		} else if ok {
			numVerified++
		}
	}

	// -1 because we self voted earlier
	if len(replies) < spec.QuorumSize-1 {
		return nil, false
	}

	spec.wg.Wait()
	numFinished := len(spec.jobs)
	for i := 0; i < numFinished; i++ {
		v := <-spec.jobs
		if v.ok {
			spec.verified[v.pc] = true
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
