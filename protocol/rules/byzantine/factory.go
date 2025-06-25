package byzantine

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

func Wrap(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	rules consensus.Ruleset,
	name string,
) (byzRules consensus.Ruleset, _ error) {
	logger.Debugf("Initializing module (byzantine strategy): %s", name)
	switch name {
	case "":
		return rules, nil // default to no byzantine strategy if no name is provided
	case NameSilence:
		byzRules = NewSilence(rules)
	case NameFork:
		byzRules = NewFork(config, blockchain, rules)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}
