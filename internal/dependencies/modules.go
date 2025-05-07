package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
	"github.com/relab/hotstuff/service/committer"
)

func newConsensusRulesModule(
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

func newByzantineStrategyModule(
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

func newCryptoModule(
	name string,
	configuration *netconfig.Config,
	logger logging.Logger,
	opts *core.Options,
) (impl modules.CryptoBase, err error) {
	switch name {
	case bls12.ModuleName:
		impl = bls12.New(configuration, logger, opts)
	case ecdsa.ModuleName:
		impl = ecdsa.New(configuration, logger, opts)
	case eddsa.ModuleName:
		impl = eddsa.New(configuration, logger, opts)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}

func newLeaderRotationModule(
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
