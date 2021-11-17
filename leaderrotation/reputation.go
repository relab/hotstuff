package leaderrotation

import (
	"fmt"

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
	if int(view) <= r.mods.Configuration().Len()+10 {
		return hotstuff.ID(view%consensus.View(r.mods.Configuration().Len()) + 1)

	}
	voters := commit_head.QuorumCert().Signature().Participants()
	voters.ForEach(func(voterID hotstuff.ID){
		currentVoter, ok := r.mods.Configuration().Replica(voterID)
		if !ok {
			r.mods.Logger().Info("Failed fetching replica", currentVoter)
		}
		thisValue := uint64(1)
		currentVoter.UpdateRep(thisValue)
		fmt.Println("the rep for ID ", currentVoter.ID()," is now:", currentVoter.GetRep())

	})
	fmt.Println("the voters were ", voters)
	return hotstuff.ID(view%consensus.View(r.mods.Configuration().Len()) + 1)
}

//NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased() consensus.LeaderRotation {
	return &repBased{}
}
