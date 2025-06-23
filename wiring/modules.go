package wiring

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/leaderrotation/carousel"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/leaderrotation/reputation"
	"github.com/relab/hotstuff/protocol/leaderrotation/roundrobin"
	"github.com/relab/hotstuff/protocol/leaderrotation/treeleader"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	"github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
)

func NewConsensusRules(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	name string,
) (rules consensus.Ruleset, err error) {
	logger.Debugf("Initializing module (consensus rules): %s", name)
	switch name {
	case "":
		fallthrough // default to chainedhotstuff if no name is provided
	case chainedhotstuff.ModuleName:
		rules = chainedhotstuff.New(logger, config, blockchain)
	case fasthotstuff.ModuleName:
		rules = fasthotstuff.New(logger, config, blockchain)
	case simplehotstuff.ModuleName:
		rules = simplehotstuff.New(logger, config, blockchain)
	default:
		return nil, fmt.Errorf("invalid consensus name: '%s'", name)
	}
	return
}

func WrapByzantineStrategy(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	rules consensus.Ruleset,
	name string,
) (byzRules consensus.Ruleset, err error) {
	logger.Debugf("Initializing module (byzantine strategy): %s", name)
	switch name {
	case "":
		return rules, nil // default to no byzantine strategy if no name is provided
	case byzantine.SilenceModuleName:
		byzRules = byzantine.NewSilence(rules)
	case byzantine.ForkModuleName:
		byzRules = byzantine.NewFork(config, blockchain, rules)
	default:
		return nil, fmt.Errorf("invalid byzantine strategy: '%s'", name)
	}
	return
}

func newCryptoModule(
	logger logging.Logger,
	config *core.RuntimeConfig,
	name string,
) (impl crypto.Base, err error) {
	logger.Debugf("Initializing module (crypto): %s", name)
	switch name {
	case "":
		fallthrough // default to ecdsa if no name is provided
	case ecdsa.ModuleName:
		impl = ecdsa.New(config)
	case bls12.ModuleName:
		impl, err = bls12.New(config)
		if err != nil {
			return nil, err
		}
	case eddsa.ModuleName:
		impl = eddsa.New(config)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}

func NewLeaderRotation(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	viewStates *protocol.ViewStates,
	name string,
	chainLength int,
) (ld leaderrotation.LeaderRotation, err error) {
	logger.Debugf("Initializing module (leader rotation): %s", name)
	switch name {
	case "":
		fallthrough // default to round-robin if no name is provided
	case roundrobin.ModuleName:
		ld = roundrobin.New(config)
	case fixedleader.ModuleName:
		ld = fixedleader.New(hotstuff.ID(1))
	case treeleader.ModuleName:
		ld = treeleader.New(config)
	case carousel.ModuleName:
		ld = carousel.New(chainLength, blockchain, viewStates, config, logger)
	case reputation.ModuleName:
		ld = reputation.New(chainLength, viewStates, config, logger)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}
