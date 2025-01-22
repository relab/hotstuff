package leaderrotation

import (
	"math/rand"
	"slices"

	wr "github.com/mroth/weightedrand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/committer"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/netconfig"
)

func init() {
	modules.RegisterModule("reputation", NewRepBased)
}

type reputationsMap map[hotstuff.ID]float64

type repBased struct {
	configuration  *netconfig.Config
	consensusRules modules.Rules
	committer      *committer.Committer
	opts           *core.Options
	logger         logging.Logger
	prevCommitHead *hotstuff.Block
	reputations    reputationsMap // latest reputations
}

// InitModule gives the module a reference to the Core object.
// It also allows the module to set module options using the OptionsBuilder
func (r *repBased) InitModule(mods *core.Core) {
	mods.Get(
		&r.configuration,
		&r.consensusRules,
		&r.committer,
		&r.opts,
		&r.logger,
	)
}

// TODO: should GetLeader be thread-safe?

// GetLeader returns the id of the leader in the given view
func (r *repBased) GetLeader(view hotstuff.View) hotstuff.ID {
	block := r.committer.CommittedBlock()
	if block.View() > view-hotstuff.View(r.consensusRules.ChainLength()) {
		// TODO: it could be possible to lookup leaders for older views if we
		// store a copy of the reputations in a metadata field of each block.
		r.logger.Error("looking up leaders of old views is not supported")
		return 0
	}

	numReplicas := r.configuration.Len()
	// use round-robin for the first few views until we get a signature
	if block.QuorumCert().Signature() == nil {
		return chooseRoundRobin(view, numReplicas)
	}

	voters := block.QuorumCert().Signature().Participants()
	numVotes := voters.Len()

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

	slices.SortFunc(weights, func(a, b wr.Choice) int {
		return int(a.Item.(hotstuff.ID) - b.Item.(hotstuff.ID))
	})

	if r.prevCommitHead.View() < block.View() {
		r.prevCommitHead = block
	}

	r.logger.Debug(weights)

	chooser, err := wr.NewChooser(weights...)
	if err != nil {
		r.logger.Error("weightedrand error: ", err)
		return 0
	}

	seed := r.opts.SharedRandomSeed() + int64(view)
	rnd := rand.New(rand.NewSource(seed))

	leader := chooser.PickSource(rnd).(hotstuff.ID)
	r.logger.Debugf("picked leader %d for view %d using seed %d", leader, view, seed)

	return leader
}

// NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased() modules.LeaderRotation {
	return &repBased{
		reputations:    make(reputationsMap),
		prevCommitHead: hotstuff.GetGenesis(),
	}
}
