package leaderrotation

import (
	"math/rand"

	"github.com/relab/hotstuff/msg"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
	"golang.org/x/exp/slices"
)

func init() {
	modules.RegisterModule("carousel", NewCarousel)
}

type carousel struct {
	mods *modules.ConsensusCore
}

func (c *carousel) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	c.mods = mods
}

func (c carousel) GetLeader(round msg.View) hotstuff.ID {
	commitHead := c.mods.Consensus().CommittedBlock()

	if commitHead.QuorumCert().Signature() == nil {
		c.mods.Logger().Debug("in startup; using round-robin")
		return chooseRoundRobin(round, c.mods.Configuration().Len())
	}

	if commitHead.BView() != round-msg.View(c.mods.Consensus().ChainLength()) {
		c.mods.Logger().Debugf("fallback to round-robin (view=%d, commitHead=%d)", round, commitHead.BView())
		return chooseRoundRobin(round, c.mods.Configuration().Len())
	}

	c.mods.Logger().Debug("proceeding with carousel")

	var (
		block       = commitHead
		f           = hotstuff.NumFaulty(c.mods.Configuration().Len())
		i           = 0
		lastAuthors = msg.NewIDSet()
		ok          = true
	)

	for ok && i < f && block != msg.GetGenesis() {
		lastAuthors.Add(block.ProposerID())
		block, ok = c.mods.BlockChain().Get(block.ParentHash())
		i++
	}

	candidates := make([]hotstuff.ID, 0, c.mods.Configuration().Len()-f)

	commitHead.QuorumCert().Signature().Participants().ForEach(func(id hotstuff.ID) {
		if !lastAuthors.Contains(id) {
			candidates = append(candidates, id)
		}
	})
	slices.Sort(candidates)

	seed := c.mods.Options().SharedRandomSeed() + int64(round)
	rnd := rand.New(rand.NewSource(seed))

	leader := candidates[rnd.Int()%len(candidates)]
	c.mods.Logger().Debugf("chose id %d", leader)

	return leader
}

// NewCarousel returns a new instance of the Carousel leader-election algorithm.
func NewCarousel() modules.LeaderRotation {
	return &carousel{}
}
