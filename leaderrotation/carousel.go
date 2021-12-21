package leaderrotation

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("carousel", NewCarousel)
}

type carousel struct {
	mods *consensus.Modules
}

func (c *carousel) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	c.mods = mods
}

func RemoveActive(active []hotstuff.ID, last []hotstuff.ID) []hotstuff.ID {
	for _, ID := range last {
		for index, IDa := range active {
			if ID == IDa {
				remove(active, index)
			}
		}

	}
	return active
}

func remove(active []hotstuff.ID, IDIndex int) []hotstuff.ID {
	active[IDIndex] = active[len(active)-1]
	return active[:len(active)-1]
}

func getActiveID(actives consensus.IDSet) []hotstuff.ID {
	myActives := []hotstuff.ID{}
	actives.ForEach(func(i hotstuff.ID) {
		myActives = append(myActives, i)
	})
	return myActives
}

func (c carousel) GetLeader(round consensus.View) hotstuff.ID {
	commit_head := c.mods.Consensus().CommittedBlock()
	if round <= 10 { //roundrobin first x views so that "particpants" is not empty
		fmt.Println("Carousel Startup")
		return hotstuff.ID(round%consensus.View(c.mods.Configuration().Len()) + 1)
	}
	endorsers := commit_head.QuorumCert().Signature().Participants()
	last_authors := []hotstuff.ID{}
	//current view vs nextRound-1
	//a blocks round must be larger than of its parent
	c.mods.Logger().Info(c.mods.Synchronizer().HighQC().View(), round-1)
	if c.mods.Synchronizer().HighQC().View() != round-1 {
		c.mods.Logger().Info("Carousel: Fallback to RoundRobin")
		return hotstuff.ID(round%consensus.View(c.mods.Configuration().Len()) + 1) //Fallback to Roundrobin
	}
	c.mods.Logger().Info(" -------------- Continued PAST RR ---------------")
	active := getActiveID(endorsers) //endorsers = voters that participated
	block := commit_head
	bc := c.mods.BlockChain()

	f := hotstuff.NumFaulty(c.mods.Configuration().Len())
	for len(last_authors) < f && block != consensus.GetGenesis() {
		last_authors = append(last_authors, block.Proposer())
		block, _ = bc.Get(block.Parent())
	}

	last_wo_active := RemoveActive(active, last_authors[:]) //active instead of 0.
	leader_candidates := last_wo_active

	s := rand.NewSource(time.Now().Unix())
	r := rand.New(s)
	n := r.Int() % len(leader_candidates)
	//fmt.Println("leader_candidates", leader_candidates)
	//fmt.Println("new leader", leader_candidates[n])
	return leader_candidates[n]
}

func NewCarousel() consensus.LeaderRotation {
	return &carousel{}
}
