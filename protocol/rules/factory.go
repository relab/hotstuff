package rules

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

func New(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	name string,
) (ruleset consensus.Ruleset, _ error) {
	logger.Debugf("Initializing module (consensus rules): %s", name)
	switch name {
	case "":
		fallthrough // default to chainedhotstuff if no name is provided
	case NameChainedHotstuff:
		ruleset = NewChainedHotStuff(logger, config, blockchain)
	case NameFastHotstuff:
		ruleset = NewFastHotstuff(logger, config, blockchain)
	case NameSimpleHotStuff:
		ruleset = NewSimpleHotStuff(logger, config, blockchain)
	default:
		return nil, fmt.Errorf("invalid consensus name: '%s'", name)
	}
	return
}
