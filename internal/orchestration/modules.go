package orchestration

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/committer"
	"github.com/relab/hotstuff/consensus/byzantine"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/consensus/fasthotstuff"
	"github.com/relab/hotstuff/consensus/simplehotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/eddsa"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/netconfig"
)

func getConsensusRules(
	name string,
	blockChain *blockchain.BlockChain,
	logger logging.Logger,
	opts *core.Options,
) (rules modules.Rules, ok bool) {
	ok = true
	switch name {
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(blockChain, logger, opts)
		break
	case simplehotstuff.ModuleName:
		rules = simplehotstuff.New(blockChain, logger)
		break
	case chainedhotstuff.ModuleName:
		rules = chainedhotstuff.New(blockChain, logger)
		break
	default:
		return nil, false
	}
	return
}

func getByzantine(
	name string,
	rules modules.Rules,
	blockChain *blockchain.BlockChain,
	synchronizer core.Synchronizer,
	opts *core.Options,
) (byz byzantine.Byzantine, ok bool) {
	ok = true
	switch name {
	case byzantine.SilenceModuleName:
		byz = byzantine.NewSilence(rules)
		break
	case byzantine.ForkModuleName:
		byz = byzantine.NewFork(rules, blockChain, synchronizer, opts)
		break
	default:
		return nil, false
	}
	return
}

func getLeaderRotation(
	name string,

	chainLength int,
	blockChain *blockchain.BlockChain,
	config *netconfig.Config,
	committer *committer.Committer,
	logger logging.Logger,
	opts *core.Options,
) (ld modules.LeaderRotation, ok bool) {
	ok = true
	switch name {
	case leaderrotation.CarouselModuleName:
		ld = leaderrotation.NewCarousel(chainLength, blockChain, config, committer, opts, logger)
		break
	case leaderrotation.ReputationModuleName:
		ld = leaderrotation.NewRepBased(chainLength, config, committer, opts, logger)
		break
	case leaderrotation.RoundRobinModuleName:
		ld = leaderrotation.NewRoundRobin(config)
		break
	case leaderrotation.FixedModuleName:
		ld = leaderrotation.NewFixed(hotstuff.ID(1))
		break
	default:
		return nil, false
	}
	return
}

func getCrypto(
	name string,
	configuration *netconfig.Config,
	logger logging.Logger,
	opts *core.Options,
) (crypto modules.CryptoBase, ok bool) {
	ok = true
	switch name {
	case bls12.ModuleName:
		crypto = bls12.New(configuration, logger, opts)
		break
	case ecdsa.ModuleName:
		crypto = ecdsa.New(configuration, logger, opts)
		break
	case eddsa.ModuleName:
		crypto = eddsa.New(configuration, logger, opts)
		break
	default:
		return nil, false
	}
	return
}

func getViewDuration(name string, opts *orchestrationpb.ReplicaOpts) (vd modules.ViewDuration) {
	return
}
