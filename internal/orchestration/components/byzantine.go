package components

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/security/blockchain"
)

func NewByzantineStrategy(
	name string,
	rules modules.ConsensusRules,
	blockChain *blockchain.BlockChain,
	opts *core.Options,
) (byzRules modules.ConsensusRules, err error) {
	switch name {
	case byzantine.SilenceModuleName:
		byzRules = byzantine.NewSilence(rules)
	case byzantine.ForkModuleName:
		byzRules = byzantine.NewFork(rules, blockChain, opts)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}
