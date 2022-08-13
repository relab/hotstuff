package leaderrotation

import (
	"math/rand"

	"github.com/relab/hotstuff/msg"

	wr "github.com/mroth/weightedrand"
	"golang.org/x/exp/slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("reputation", NewRepBased)
}

type reputationsMap map[hotstuff.ID]float64

type repBased struct {
	mods           *modules.ConsensusCore
	prevCommitHead *msg.Block
	reputations    reputationsMap // latest reputations
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder
func (r *repBased) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	r.mods = mods
}

// TODO: should GetLeader be thread-safe?

// GetLeader returns the id of the leader in the given view
func (r *repBased) GetLeader(view msg.View) hotstuff.ID {
	block := r.mods.Consensus().CommittedBlock()
	if block.View() > view-msg.View(r.mods.Consensus().ChainLength()) {
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
		weights = append(weights, wr.Choice{
			Item:   voterID,
			Weight: uint(r.reputations[voterID] * 10),
		})
	})

	slices.SortFunc(weights, func(a, b wr.Choice) bool {
		return a.Item.(hotstuff.ID) >= b.Item.(hotstuff.ID)
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
func NewRepBased() modules.LeaderRotation {
	return &repBased{
		reputations:    make(reputationsMap),
		prevCommitHead: msg.GetGenesis(),
	}
}
