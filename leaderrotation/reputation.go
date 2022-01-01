package leaderrotation

import (
	"math/rand"
	"sort"

	wr "github.com/mroth/weightedrand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("reputation", NewRepBased)
}

type reputationsMap map[hotstuff.ID]float64

type repBased struct {
	mods           *consensus.Modules
	prevCommitHead *consensus.Block
	reputations    reputationsMap // latest reputations
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder
func (r *repBased) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	r.mods = mods
}

// TODO: should GetLeader be thread-safe?

// GetLeader returns the id of the leader in the given view
func (r *repBased) GetLeader(view consensus.View) hotstuff.ID {
	block := r.mods.Consensus().CommittedBlock()
	if block.View() > view-consensus.View(r.mods.Consensus().ChainLength()) {
		// TODO: it could be possible to lookup leaders for older views if we
		// store a copy of the reputations in a metadata field of each block.
		r.mods.Logger().Error("looking up leaders of old views is not supported")
		return 0
	}

	numReplicas := r.mods.Configuration().Len()
	// use round-robin for the first few views until we get a signature
	if block.QuorumCert().Signature() == nil {
		return chooseRoundRobin(view, numReplicas)
	}

	voters := block.QuorumCert().Signature().Participants()
	numVotes := 0
	voters.ForEach(func(hotstuff.ID) {
		numVotes++
	})

	frac := float64((2.0 / 3.0) * float64(numReplicas))
	reputation := ((float64(numVotes) - frac) / frac)

	weights := make([]wr.Choice, 0, numVotes)
	voters.ForEach(func(voterID hotstuff.ID) {
		// we should only update the reputations once for each commit head.
		if r.prevCommitHead.View() < block.View() {
			r.reputations[voterID] += reputation
		}
		i := sort.Search(len(weights), func(i int) bool { return weights[i].Item.(hotstuff.ID) >= voterID })
		weights = append(weights[:i+1], weights[i:]...)
		weights[i] = wr.Choice{
			Item:   voterID,
			Weight: uint(r.reputations[voterID] * 10),
		}
	})

	if r.prevCommitHead.View() < block.View() {
		r.prevCommitHead = block
	}

	r.mods.Logger().Debug(weights)

	chooser, err := wr.NewChooser(weights...)
	if err != nil {
		r.mods.Logger().Error("weightedrand error: ", err)
		return 0
	}

	seed := r.mods.Options().SharedRandomSeed() + int64(view)
	rnd := rand.New(rand.NewSource(seed))

	leader := chooser.PickSource(rnd).(hotstuff.ID)
	r.mods.Logger().Debugf("picked leader %d for view %d using seed %d", leader, view, seed)

	return leader
}

// NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased() consensus.LeaderRotation {
	return &repBased{
		reputations:    make(reputationsMap),
		prevCommitHead: consensus.GetGenesis(),
	}
}
