// Package leaderrotation provide various leader rotation algorithms.
package carousel

import (
	"math/rand"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/committer"
)

const ModuleName = "carousel"

type carousel struct {
	blockChain   *blockchain.BlockChain
	committer    *committer.Committer
	config       *core.RuntimeConfig
	logger       logging.Logger
	viewDuration modules.ViewDuration

	chainLength int
}

// New returns a new instance of the Carousel leader-election algorithm.
func New(
	chainLength int,
	vdParams viewduration.Params,

	blockChain *blockchain.BlockChain,
	committer *committer.Committer,
	config *core.RuntimeConfig,
	logger logging.Logger,
) modules.LeaderRotation {
	return &carousel{
		blockChain:   blockChain,
		chainLength:  chainLength,
		committer:    committer,
		config:       config,
		logger:       logger,
		viewDuration: viewduration.NewDynamic(vdParams),
	}
}

func (c carousel) GetLeader(round hotstuff.View) hotstuff.ID {
	commitHead := c.committer.CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.logger.Debug("in startup; using round-robin")
		return leaderrotation.ChooseRoundRobin(round, c.config.ReplicaCount())
	}

	if commitHead.View() != round-hotstuff.View(c.chainLength) {
		c.logger.Debugf("fallback to round-robin (view=%d, commitHead=%d)", round, commitHead.View())
		return leaderrotation.ChooseRoundRobin(round, c.config.ReplicaCount())
	}

	c.logger.Debug("proceeding with carousel")

	var (
		block       = commitHead
		f           = hotstuff.NumFaulty(c.config.ReplicaCount())
		i           = 0
		lastAuthors = hotstuff.NewIDSet()
		ok          = true
	)

	for ok && i < f && block != hotstuff.GetGenesis() {
		lastAuthors.Add(block.Proposer())
		block, ok = c.blockChain.Get(block.Parent())
		i++
	}

	candidates := make([]hotstuff.ID, 0, c.config.ReplicaCount()-f)

	commitHead.QuorumCert().Signature().Participants().ForEach(func(id hotstuff.ID) {
		if !lastAuthors.Contains(id) {
			candidates = append(candidates, id)
		}
	})
	slices.Sort(candidates)

	seed := c.config.SharedRandomSeed() + int64(round)
	rnd := rand.New(rand.NewSource(seed))

	leader := candidates[rnd.Int()%len(candidates)]
	c.logger.Debugf("chose id %d", leader)

	return leader
}

func (c carousel) ViewDuration() modules.ViewDuration {
	return c.viewDuration
}
