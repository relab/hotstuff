package leaderrotation

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/blockchain"
)

func New(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	viewStates *protocol.ViewStates,
	name string,
	chainLength int,
) (ld LeaderRotation, _ error) {
	switch name {
	case "":
		fallthrough // default to round-robin if no name is provided
	case NameRoundRobin:
		ld = NewRoundRobin(config)
	case NameFixed:
		ld = NewFixed(hotstuff.ID(1))
	case NameTree:
		ld = NewTreeBased(config)
	case NameCarousel:
		ld = NewCarousel(chainLength, blockchain, viewStates, config, logger)
	case NameReputation:
		ld = NewRepBased(chainLength, viewStates, config, logger)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}
