package components

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/committer"
)

func NewLeaderRotation(
	name string,
	chainLength int,
	blockChain *blockchain.BlockChain,
	config *netconfig.Config,
	committer *committer.Committer,
	logger logging.Logger,
	opts *core.Options,
) (ld modules.LeaderRotation, err error) {
	switch name {
	case leaderrotation.CarouselModuleName:
		ld = leaderrotation.NewCarousel(chainLength, blockChain, config, committer, opts, logger)
	case leaderrotation.ReputationModuleName:
		ld = leaderrotation.NewRepBased(chainLength, config, committer, opts, logger)
	case leaderrotation.RoundRobinModuleName:
		ld = leaderrotation.NewRoundRobin(config)
	case leaderrotation.FixedModuleName:
		ld = leaderrotation.NewFixed(hotstuff.ID(1))
	case leaderrotation.TreeLeaderModuleName:
		ld = leaderrotation.NewTreeLeader(opts)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}
