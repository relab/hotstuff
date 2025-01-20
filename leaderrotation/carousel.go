// Package leaderrotation provide various leader rotation algorithms.
package leaderrotation

import (
	"math/rand"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("carousel", NewCarousel)
}

type carousel struct {
	comps core.ComponentList
}

func (c *carousel) InitComponent(mods *core.Core) {
	c.comps = mods.Components()
}

func (c carousel) GetLeader(round hotstuff.View) hotstuff.ID {
	commitHead := c.comps.Consensus.CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.comps.Logger.Debug("in startup; using round-robin")
		return chooseRoundRobin(round, c.comps.Configuration.Len())
	}

	if commitHead.View() != round-hotstuff.View(c.comps.Consensus.ChainLength()) {
		c.comps.Logger.Debugf("fallback to round-robin (view=%d, commitHead=%d)", round, commitHead.View())
		return chooseRoundRobin(round, c.comps.Configuration.Len())
	}

	c.comps.Logger.Debug("proceeding with carousel")

	var (
		block       = commitHead
		f           = hotstuff.NumFaulty(c.comps.Configuration.Len())
		i           = 0
		lastAuthors = hotstuff.NewIDSet()
		ok          = true
	)

	for ok && i < f && block != hotstuff.GetGenesis() {
		lastAuthors.Add(block.Proposer())
		block, ok = c.comps.BlockChain.Get(block.Parent())
		i++
	}

	candidates := make([]hotstuff.ID, 0, c.comps.Configuration.Len()-f)

	commitHead.QuorumCert().Signature().Participants().ForEach(func(id hotstuff.ID) {
		if !lastAuthors.Contains(id) {
			candidates = append(candidates, id)
		}
	})
	slices.Sort(candidates)

	seed := c.comps.Options.SharedRandomSeed() + int64(round)
	rnd := rand.New(rand.NewSource(seed))

	leader := candidates[rnd.Int()%len(candidates)]
	c.comps.Logger.Debugf("chose id %d", leader)

	return leader
}

// NewCarousel returns a new instance of the Carousel leader-election algorithm.
func NewCarousel() modules.LeaderRotation {
	return &carousel{}
}
