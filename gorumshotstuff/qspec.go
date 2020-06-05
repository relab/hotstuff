package gorumshotstuff

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
)

type hotstuffQSpec struct {
	*hotstuff.SignatureCache
	*hotstuff.ReplicaConfig
	verified map[*proto.PartialCert]bool
	jobs     chan struct {
		pc *proto.PartialCert
		ok bool
	}
	wg sync.WaitGroup
}

func (qspec *hotstuffQSpec) Reset() {
	qspec.verified = make(map[*proto.PartialCert]bool)
	qspec.jobs = make(chan struct {
		pc *proto.PartialCert
		ok bool
	}, len(qspec.Replicas))
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (qspec *hotstuffQSpec) ProposeQF(req *proto.Block, replies map[uint32]*proto.PartialCert) (*proto.QuorumCert, bool) {
	numVerified := 0
	for _, pc := range replies {
		if ok, verifiedBefore := qspec.verified[pc]; !verifiedBefore {
			qspec.verified[pc] = false
			qspec.wg.Add(1)
			go func(pc *proto.PartialCert) {
				cert := pc.FromProto()
				ok := qspec.VerifySignature(cert.Sig, cert.BlockHash)
				qspec.jobs <- struct {
					pc *proto.PartialCert
					ok bool
				}{pc, ok}
				qspec.wg.Done()
			}(pc)
		} else if ok {
			numVerified++
		}
	}

	// -1 because we self voted earlier
	if len(replies) < qspec.QuorumSize-1 {
		return nil, false
	}

	qspec.wg.Wait()
	numFinished := len(qspec.jobs)
	for i := 0; i < numFinished; i++ {
		v := <-qspec.jobs
		if v.ok {
			qspec.verified[v.pc] = true
			numVerified++
		}
	}

	if numVerified >= qspec.QuorumSize-1 {
		qc := hotstuff.CreateQuorumCert(req.FromProto())
		for c, ok := range qspec.verified {
			if ok {
				qc.AddPartial(c.FromProto())
			}
		}
		return proto.QuorumCertToProto(qc), true
	}

	return nil, false
}
