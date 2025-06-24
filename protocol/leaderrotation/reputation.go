package leaderrotation

import (
	"math/rand"
	"slices"

	wr "github.com/mroth/weightedrand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
)

const ModuleNameReputation = "reputation"

type reputationsMap map[hotstuff.ID]float64

type RepBased struct {
	viewStates *protocol.ViewStates
	config     *core.RuntimeConfig
	logger     logging.Logger

	chainLength    int
	prevCommitHead *hotstuff.Block
	reputations    reputationsMap // latest reputations
}

// TODO: should GetLeader be thread-safe?

// GetLeader returns the id of the leader in the given view
func (r *RepBased) GetLeader(view hotstuff.View) hotstuff.ID {
	block := r.viewStates.CommittedBlock()
	if block.View() > view-hotstuff.View(r.chainLength) {
		// TODO: it could be possible to lookup leaders for older views if we
		// store a copy of the reputations in a metadata field of each block.
		r.logger.Error("looking up leaders of old views is not supported")
		return 0
	}

	numReplicas := r.config.ReplicaCount()
	// use round-robin for the first few views until we get a signature
	if block.QuorumCert().Signature() == nil {
		return ChooseRoundRobin(view, numReplicas)
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

	seed := r.config.SharedRandomSeed() + int64(view)
	rnd := rand.New(rand.NewSource(seed))

	leader := chooser.PickSource(rnd).(hotstuff.ID)
	r.logger.Debugf("picked leader %d for view %d using seed %d", leader, view, seed)

	return leader
}

// NewRepBased returns a new random reputation-based leader rotation implementation
func NewRepBased(
	chainLength int,

	viewStates *protocol.ViewStates,
	config *core.RuntimeConfig,
	logger logging.Logger,
) *RepBased {
	return &RepBased{
		viewStates: viewStates,
		config:     config,
		logger:     logger,

		chainLength:    chainLength,
		reputations:    make(reputationsMap),
		prevCommitHead: hotstuff.GetGenesis(),
	}
}

var _ LeaderRotation = (*RepBased)(nil)
