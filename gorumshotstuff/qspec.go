package gorumshotstuff

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff/internal/proto"
)

// HotstuffQSpec is a spesific qspec used in hotstuff.
type HotstuffQSpec struct {
	*hotstuff.SignatureCache
	*hotstuff.ReplicaConfig
	verified map[*proto.PartialCert]bool
	jobs     chan struct {
		pc *proto.PartialCert
		ok bool
		id hotstuff.ReplicaID
	}
	wg            sync.WaitGroup
	latestReplies map[uint32]*proto.PartialCert
	vHeight       int
	bestReplicaID hotstuff.ReplicaID
}

// Reset resets qspec state.
func (qspec *HotstuffQSpec) Reset() {
	qspec.verified = make(map[*proto.PartialCert]bool)
	qspec.jobs = make(chan struct {
		pc *proto.PartialCert
		ok bool
		id hotstuff.ReplicaID
	}, len(qspec.Replicas))
}

// GetLatestReplies retruns the latest nodeID-reply map that has been recived in ProposeQF along with the height it got this map on.
func (qspec *HotstuffQSpec) GetLatestReplies() (int, map[uint32]*proto.PartialCert) {
	return qspec.vHeight, qspec.latestReplies
}

// FlushBestReplica set the bestReplica veriable to 0. This should be call at the end of every round of proposals by the leader.
func (qspec *HotstuffQSpec) FlushBestReplica() {
	qspec.bestReplicaID = 0
}

// GetBestReplica returns the ID of the replica which replied first with a valid partial certificate.
func (qspec *HotstuffQSpec) GetBestReplica() hotstuff.ReplicaID {
	logger.Println("hi")
	logger.Println(qspec)
	return qspec.bestReplicaID
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (qspec *HotstuffQSpec) ProposeQF(req *proto.BlockAndLeaderID, replies map[uint32]*proto.PartialCert) (*proto.QuorumCert, bool) {
	numVerified := 0
	for id, pc := range replies {
		if ok, verifiedBefore := qspec.verified[pc]; !verifiedBefore {
			qspec.verified[pc] = false
			qspec.wg.Add(1)
			go func(pc *proto.PartialCert, id uint32) {
				cert := pc.FromProto()
				ok := qspec.VerifySignature(cert.Sig, cert.BlockHash)
				qspec.jobs <- struct {
					pc *proto.PartialCert
					ok bool
					id hotstuff.ReplicaID
				}{pc, ok, hotstuff.ReplicaID(id)}
				qspec.wg.Done()
			}(pc, id)
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
			if i == 0 {
				logger.Println("hello")
				logger.Println(v.id)
				qspec.bestReplicaID = v.id
				logger.Println(qspec.bestReplicaID)
				logger.Println(qspec.GetBestReplica())
			}
			qspec.verified[v.pc] = true
			numVerified++
		}
	}

	if numVerified >= qspec.QuorumSize-1 {
		qc := hotstuff.CreateQuorumCert(req.Block.FromProto())
		for c, ok := range qspec.verified {
			if ok {
				qc.AddPartial(c.FromProto())
			}
		}
		qspec.latestReplies = replies
		qspec.vHeight = int(req.Block.Height)
		return proto.QuorumCertToProto(qc), true
	}

	return nil, false
}
