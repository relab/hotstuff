package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/leaderrotation/carousel"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/leaderrotation/reputation"
	"github.com/relab/hotstuff/protocol/leaderrotation/roundrobin"
	"github.com/relab/hotstuff/protocol/leaderrotation/treeleader"
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

func NewConsensusRules(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	name string,
) (rules modules.ConsensusRules, err error) {
	logger.Debugf("Initializing module (consensus rules): %s", name)
	switch name {
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(logger, config, blockChain)
	case simplehotstuff.ModuleName:
		rules = simplehotstuff.New(logger, blockChain)
	case chainedhotstuff.ModuleName:
		rules = chainedhotstuff.New(logger, blockChain)
	default:
		return nil, fmt.Errorf("invalid consensus name: '%s'", name)
	}
	return
}

func WrapByzantineStrategy(
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	rules modules.ConsensusRules,
	name string,
) (byzRules modules.ConsensusRules, err error) {
	// logger.Debugf("Initializing module (byzantine strategy): %s", name)
	switch name {
	case byzantine.SilenceModuleName:
		byzRules = byzantine.NewSilence(rules)
	case byzantine.ForkModuleName:
		byzRules = byzantine.NewFork(rules, blockChain, config)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}

func newCryptoModule(
	logger logging.Logger,
	config *core.RuntimeConfig,
	name string,
) (impl modules.CryptoBase, err error) {
	logger.Debugf("Initializing module (crypto): %s", name)
	switch name {
	case bls12.ModuleName:
		impl = bls12.New(logger, config)
	case ecdsa.ModuleName:
		impl = ecdsa.New(logger, config)
	case eddsa.ModuleName:
		impl = eddsa.New(logger, config)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}

func NewLeaderRotation(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	committer *committer.Committer,
	vdParams viewduration.Params,
	name string,
	chainLength int,
) (ld modules.LeaderRotation, err error) {
	logger.Debugf("Initializing module (leader rotation): %s", name)
	switch name {
	case carousel.ModuleName:
		ld = carousel.New(chainLength, vdParams, blockChain, committer, config, logger)
	case reputation.ModuleName:
		ld = reputation.New(chainLength, vdParams, committer, config, logger)
	case roundrobin.ModuleName:
		ld = roundrobin.New(config, vdParams)
	case fixedleader.ModuleName:
		ld = fixedleader.New(hotstuff.ID(1), vdParams)
	case treeleader.ModuleName:
		ld = treeleader.New(vdParams.StartTimeout(), config)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}
