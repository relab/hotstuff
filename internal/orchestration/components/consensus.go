package components

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/security/blockchain"
)

func NewConsensusRules(
	name string,
	blockChain *blockchain.BlockChain,
	logger logging.Logger,
	opts *core.Options,
) (rules modules.ConsensusRules, err error) {
	switch name {
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(blockChain, logger, opts)
	case simplehotstuff.ModuleName:
		rules = simplehotstuff.New(blockChain, logger)
	case chainedhotstuff.ModuleName:
		rules = chainedhotstuff.New(blockChain, logger)
	default:
		return nil, fmt.Errorf("invalid consensus name: '%s'", name)
	}
	return
}
