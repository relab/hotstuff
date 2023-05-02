// Package leaderrotation provide various leader rotation algorithms.
package leaderrotation

import (
	"math/rand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"golang.org/x/exp/slices"
)

func init() {
	modules.RegisterModule("carousel", NewCarousel)
}

type carousel struct {
	blockChain    modules.BlockChain
	configuration modules.Configuration
	consensus     modules.Consensus
	opts          *modules.Options
	logger        logging.Logger
}

func (c *carousel) InitModule(mods *modules.Core) {
	mods.Get(
		&c.blockChain,
		&c.configuration,
		&c.consensus,
		&c.opts,
		&c.logger,
	)
}

func (c carousel) GetLeader(round hotstuff.View) hotstuff.ID {
	commitHead := c.consensus.CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.logger.Debug("in startup; using round-robin")
		return chooseRoundRobin(round, c.configuration.Len())
	}

	if commitHead.View() != round-hotstuff.View(c.consensus.ChainLength()) {
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
func NewCarousel() modules.LeaderRotation {
	return &carousel{}
}
