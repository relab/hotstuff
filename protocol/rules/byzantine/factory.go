package byzantine

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

func Wrap(
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	rules consensus.Ruleset,
	name string,
) (byzRules consensus.Ruleset, _ error) {
	switch name {
	case "":
		return rules, nil // default to no byzantine strategy if no name is provided
	case NameSilence:
		byzRules = NewSilence(rules)
	case NameFork:
		byzRules = NewFork(config, blockchain, rules)
	case NameIncreaseView:
		byzRules = NewIncreaseView(config, blockchain, rules)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}
