package leaderrotation

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"

	wr "github.com/mroth/weightedrand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type repBased struct {
	mods        *consensus.Modules
	replicaList []wr.Choice
}

//InitConsensusModule gives the module a reference to the Modules object.
//It also allows the module to set module options using the OptionsBuilder
func (r *repBased) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	r.mods = mods
	r.replicaList = []wr.Choice{}
}

//GetLeader returns the id of the leader in the given view
func (r repBased) GetLeader(view consensus.View) hotstuff.ID {
	commit_head := r.mods.Consensus().CommittedBlock() //fetch previous comitted block
	numReplicas := r.mods.Configuration().Len()
	blockHash := r.mods.Consensus().CommittedBlock().Hash().String()
	h := fnv.New32a()
	h.Write([]byte(blockHash))
	hashInt := h.Sum32()
	rand.Seed(int64(hashInt))
	
	//fmt.Println("view", view, "my id",myId,  "commitQC", commit_head.QuorumCert())
	if int(view) <= numReplicas+10 {
		return hotstuff.ID(view%consensus.View(numReplicas) + 1)
	}

	voters := commit_head.QuorumCert().Signature().Participants()
	numVotes := 1.0 //is 1 because leader counts as a vote
	voters.ForEach(func(hotstuff.ID) {
		numVotes += 1.0
	})
	frac := float64((2.0 / 3.0) * float64(numReplicas))
	reputation := ((numVotes - frac) / frac)
	voters.ForEach(func(voterID hotstuff.ID) {
		
		currentVoter, ok := r.mods.Configuration().Replica(voterID)
		if !ok {
			r.mods.Logger().Info("Failed fetching current replica", currentVoter)
		}
		if currentVoter.ID() == voterID {
			
/* 			if int(r.mods.ID()) == 2 {
				fmt.Println("updating rep", voterID, " in view", view, " iam ", r.mods.ID())
			}
			fmt.Println("should?", shouldUpdate) */
				currentVoter.UpdateRep(reputation)
			
		} 
		//fmt.Println("before", candidate.ID(), candidate.GetRep())
		r.replicaList = append(r.replicaList, wr.Choice{Item: strconv.Itoa(int(currentVoter.ID())), Weight: uint(currentVoter.GetRep()*10)}) 
	})
	//fmt.Println("choices", r.replicaList)
	chooser, err := wr.NewChooser(r.replicaList...)
	if err != nil {
		fmt.Println(err)
	}
	resultLeader := chooser.Pick().(string)
	intLeader, _ := strconv.Atoi(resultLeader)

	return hotstuff.ID(intLeader)
}

//NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased() consensus.LeaderRotation {
	return &repBased{}
}
