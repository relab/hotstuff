package replicaranking

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/rankingpb"
)

type ReplicaRanking struct {
	scores map[hotstuff.ID]float64
}

func (rr *ReplicaRanking) handleComplaint(complaint *rankingpb.Complaint) {

}
