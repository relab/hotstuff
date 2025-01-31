// Package leaderrotation provide various leader rotation algorithms.
package leaderrotation

import (
	"math/rand"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/committer"
)

const CarouselModuleName = "carousel"

type carousel struct {
	blockChain    *blockchain.BlockChain
	configuration *netconfig.Config
	committer     *committer.Committer
	opts          *core.Options
	logger        logging.Logger

	chainLength int
}

func (c carousel) GetLeader(round hotstuff.View) hotstuff.ID {
	commitHead := c.committer.CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.logger.Debug("in startup; using round-robin")
		return chooseRoundRobin(round, c.configuration.Len())
	}

	if commitHead.View() != round-hotstuff.View(c.chainLength) {
		c.logger.Debugf("fallback to round-robin (view=%d, commitHead=%d)", round, commitHead.View())
		return chooseRoundRobin(round, c.configuration.Len())
	}

	c.logger.Debug("proceeding with carousel")

	var (
		block       = commitHead
		f           = hotstuff.NumFaulty(c.configuration.Len())
		i           = 0
		lastAuthors = hotstuff.NewIDSet()
		ok          = true
	)

	for ok && i < f && block != hotstuff.GetGenesis() {
		lastAuthors.Add(block.Proposer())
		block, ok = c.blockChain.Get(block.Parent())
		i++
	}

	candidates := make([]hotstuff.ID, 0, c.configuration.Len()-f)

	commitHead.QuorumCert().Signature().Participants().ForEach(func(id hotstuff.ID) {
		if !lastAuthors.Contains(id) {
			candidates = append(candidates, id)
		}
	})
	slices.Sort(candidates)

	seed := c.opts.SharedRandomSeed() + int64(round)
	rnd := rand.New(rand.NewSource(seed))

	leader := candidates[rnd.Int()%len(candidates)]
	c.logger.Debugf("chose id %d", leader)

	return leader
}

// NewCarousel returns a new instance of the Carousel leader-election algorithm.
func NewCarousel(
	chainLength int,

	blockChain *blockchain.BlockChain,
	configuration *netconfig.Config,
	committer *committer.Committer,
	opts *core.Options,
	logger logging.Logger,
) modules.LeaderRotation {
	return &carousel{
		blockChain:    blockChain,
		chainLength:   chainLength,
		configuration: configuration,
		committer:     committer,
		opts:          opts,
		logger:        logger,
	}
}
