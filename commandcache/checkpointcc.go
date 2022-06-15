package commandcache

import (
	"context"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// checkPointCmdCache piggybacks the command cache implementation to also provide checkpoint service for the replicas.
// Though it is implemented as separate file to the existing commandcache, it is planned to merge them.
type checkPointCmdCache struct {
	mods                       *consensus.Modules
	cmdCache                   *cmdCache
	checkPointRotationIndex    int
	highestCheckPointViewIndex uint64
}

func NewCC(cc *cmdCache, checkPointIndex int) *checkPointCmdCache {
	return &checkPointCmdCache{
		cmdCache:                cc,
		checkPointRotationIndex: checkPointIndex,
	}
}

// InitModule gives the module access to the other modules.
func (c *checkPointCmdCache) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	c.mods = mods
}

func (c *checkPointCmdCache) AddCommand(cmd *clientpb.Command) {
	c.cmdCache.AddCommand(cmd)
}

// Get returns a batch of commands to propose.
func (c *checkPointCmdCache) Get(ctx context.Context) (cmd consensus.Command, ok bool) {
	view := uint64(c.mods.Synchronizer().View())
	batch, ok := c.cmdCache.getBatch(ctx)
	if !ok {
		return cmd, ok
	}
	if view%uint64(c.checkPointRotationIndex) == 0 ||
		view-uint64(c.checkPointRotationIndex) > c.highestCheckPointViewIndex {
		batch.IsCheckPointRequest = true
		view = view - (view % uint64(c.checkPointRotationIndex))
		batch.CheckPointViewNumber = uint64(view)
	}
	return c.cmdCache.marshalBatch(batch)
}

// Accept returns true if the replica can accept the batch.
func (c *checkPointCmdCache) Accept(cmd consensus.Command) bool {
	batch, ok := c.cmdCache.unmarshalCommand(cmd)
	if !ok {
		c.mods.Logger().Info("Returning accept false")
		return false
	}
	if batch.IsCheckPointRequest && batch.CheckPointViewNumber > c.highestCheckPointViewIndex {
		c.mods.BlockChain().CreateSnapShot()
		c.highestCheckPointViewIndex = batch.CheckPointViewNumber
	}
	return c.cmdCache.Accept(cmd)
}

// Proposed updates the serial numbers such that we will not accept the given batch again.
func (c *checkPointCmdCache) Proposed(cmd consensus.Command) {
	c.cmdCache.Proposed(cmd)
}

func (c *checkPointCmdCache) GetHighestCheckPointedView() consensus.View {
	return consensus.View(c.highestCheckPointViewIndex)
}

var _ consensus.Acceptor = (*checkPointCmdCache)(nil)
var _ consensus.CommandQueue = (*checkPointCmdCache)(nil)
