package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
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
	globals *globals.Globals,
) (rules modules.ConsensusRules, err error) {
	logger.Debugf("Initializing module (consensus rules): %s", name)
	switch name {
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(blockChain, logger, globals)
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
	globals *globals.Globals,
) (byzRules modules.ConsensusRules, err error) {
	// logger.Debugf("Initializing module (byzantine strategy): %s", name)
	switch name {
	case byzantine.SilenceModuleName:
		byzRules = byzantine.NewSilence(rules)
	case byzantine.ForkModuleName:
		byzRules = byzantine.NewFork(rules, blockChain, globals)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}

func newCryptoModule(
	name string,
	logger logging.Logger,
	globals *globals.Globals,
) (impl modules.CryptoBase, err error) {
	logger.Debugf("Initializing module (crypto): %s", name)
	switch name {
	case bls12.ModuleName:
		impl = bls12.New(logger, globals)
	case ecdsa.ModuleName:
		impl = ecdsa.New(logger, globals)
	case eddsa.ModuleName:
		impl = eddsa.New(logger, globals)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}

func newLeaderRotationModule(
	name string,
	chainLength int,
	vdParams viewduration.Params,
	blockChain *blockchain.BlockChain,
	committer *committer.Committer,
	logger logging.Logger,
	globals *globals.Globals,
) (ld modules.LeaderRotation, err error) {
	logger.Debugf("Initializing module (leader rotation): %s", name)
	switch name {
	case leaderrotation.CarouselModuleName:
		ld = leaderrotation.NewCarousel(chainLength, vdParams, blockChain, committer, globals, logger)
	case leaderrotation.ReputationModuleName:
		ld = leaderrotation.NewRepBased(chainLength, vdParams, committer, globals, logger)
	case leaderrotation.RoundRobinModuleName:
		ld = leaderrotation.NewRoundRobin(globals, vdParams)
	case leaderrotation.FixedModuleName:
		ld = leaderrotation.NewFixed(hotstuff.ID(1), vdParams)
	case leaderrotation.TreeLeaderModuleName:
		ld = leaderrotation.NewTreeLeader(vdParams.StartTimeout(), globals)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}
