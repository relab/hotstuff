// Package leaderrotation provide various leader rotation algorithms.
package leaderrotation

import (
	"math/rand"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/blockchain"
)

const ModuleNameCarousel = "carousel"

type Carousel struct {
	blockchain *blockchain.Blockchain
	viewStates *protocol.ViewStates
	config     *core.RuntimeConfig
	logger     logging.Logger

	chainLength int
}

// NewCarousel returns a new instance of the Carousel leader-election algorithm.
func NewCarousel(
	chainLength int,

	blockchain *blockchain.Blockchain,
	viewStates *protocol.ViewStates,
	config *core.RuntimeConfig,
	logger logging.Logger,
) *Carousel {
	return &Carousel{
		blockchain:  blockchain,
		chainLength: chainLength,
		viewStates:  viewStates,
		config:      config,
		logger:      logger,
	}
}

func (c *Carousel) GetLeader(round hotstuff.View) hotstuff.ID {
	commitHead := c.viewStates.CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.logger.Debug("in startup; using round-robin")
		return ChooseRoundRobin(round, c.config.ReplicaCount())
	}

	if commitHead.View() != round-hotstuff.View(c.chainLength) {
		c.logger.Debugf("fallback to round-robin (view=%d, commitHead=%d)", round, commitHead.View())
		return ChooseRoundRobin(round, c.config.ReplicaCount())
	}

	c.logger.Debug("proceeding with carousel")

	var (
		block       = commitHead
		genesis     = hotstuff.GetGenesis()
		f           = hotstuff.NumFaulty(c.config.ReplicaCount())
		lastAuthors = make([]hotstuff.ID, 0, f)
		ok          = true
	)
	for i := 0; ok && i < f && block != genesis; i++ {
		lastAuthors = append(lastAuthors, block.Proposer())
		block, ok = c.blockchain.Get(block.Parent())
	}

	candidates := make([]hotstuff.ID, 0, c.config.ReplicaCount()-f)

	commitHead.QuorumCert().Signature().Participants().ForEach(func(id hotstuff.ID) {
		if !slices.Contains(lastAuthors, id) {
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

var _ LeaderRotation = (*Carousel)(nil)
