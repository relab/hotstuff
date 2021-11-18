package leaderrotation

import (
	"fmt"
	"hash/fnv"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type repBased struct {
	mods *consensus.Modules
}

//InitConsensusModule gives the module a reference to the Modules object.
//It also allows the module to set module options using the OptionsBuilder
func (r *repBased) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	r.mods = mods

}

//GetLeader returns the id of the leader in the given view
func (r repBased) GetLeader(view consensus.View) hotstuff.ID {
	commit_head := r.mods.Consensus().CommittedBlock() //fetch previous comitted block
	numReplicas := r.mods.Configuration().Len() 
	blockHash := r.mods.Consensus().CommittedBlock().Hash().String()
	h := fnv.New32a()
	h.Write([]byte(blockHash))
	hashInt := h.Sum32()

	fmt.Println("the block hash", hashInt)
	if int(view) <= numReplicas+10 {
		return hotstuff.ID(view%consensus.View(numReplicas) + 1)
	}
	voters := commit_head.QuorumCert().Signature().Participants()
	numVotes := 1.0 //is 1 because leader counts as a vote
	voters.ForEach(func(hotstuff.ID) {
		numVotes += 1.0
	})
	frac := float64((2.0/3.0)*float64(numReplicas))
	reputation := ((numVotes - frac) / frac)
	fmt.Println("the reputation is", reputation)
	voters.ForEach(func(voterID hotstuff.ID) {

		currentVoter, ok := r.mods.Configuration().Replica(voterID)
		if !ok {
			r.mods.Logger().Info("Failed fetching replica", currentVoter)
		}
		currentVoter.UpdateRep(reputation)
		fmt.Println("the rep for ID ", currentVoter.ID(), " is now:", currentVoter.GetRep())

	})
	return hotstuff.ID(view%consensus.View(r.mods.Configuration().Len()) + 1)
}

//NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased() consensus.LeaderRotation {
	return &repBased{}
}
