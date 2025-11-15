package twins

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/blockchain"
)

func newTwinsConsensusRules(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	name string,
) (ruleset consensus.Ruleset, err error) {
	switch name {
	case nameVulnerableFHS:
		ruleset = NewVulnFHS(logger, blockchain,
			rules.NewFastHotStuff(logger, config, blockchain),
		)
	default:
		ruleset, err = rules.New(logger, config, blockchain, name)
	}
	return
}
