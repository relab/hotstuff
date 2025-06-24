package wiring

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/comm/clique"
	"github.com/relab/hotstuff/protocol/comm/kauri"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
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
) (ruleset consensus.Ruleset, _ error) {
	logger.Debugf("Initializing module (consensus rules): %s", name)
	switch name {
	case "":
		fallthrough // default to chainedhotstuff if no name is provided
	case rules.ModuleNameChainedHotstuff:
		ruleset = rules.NewChainedHotStuff(logger, config, blockchain)
	case rules.ModuleNameFastHotstuff:
		ruleset = rules.NewFastHotstuff(logger, config, blockchain)
	case rules.ModuleNameSimpleHotStuff:
		ruleset = rules.NewSimpleHotStuff(logger, config, blockchain)
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
) (byzRules consensus.Ruleset, _ error) {
	logger.Debugf("Initializing module (byzantine strategy): %s", name)
	switch name {
	case "":
		return rules, nil // default to no byzantine strategy if no name is provided
	case byzantine.ModuleNameSilence:
		byzRules = byzantine.NewSilence(rules)
	case byzantine.ModuleNameFork:
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
) (ld leaderrotation.LeaderRotation, _ error) {
	logger.Debugf("Initializing module (leader rotation): %s", name)
	switch name {
	case "":
		fallthrough // default to round-robin if no name is provided
	case leaderrotation.ModuleNameRoundRobin:
		ld = leaderrotation.NewRoundRobin(config)
	case leaderrotation.ModuleNameFixed:
		ld = leaderrotation.NewFixed(hotstuff.ID(1))
	case leaderrotation.ModuleNameTree:
		ld = leaderrotation.NewTreeBased(config)
	case leaderrotation.ModuleNameCarousel:
		ld = leaderrotation.NewCarousel(chainLength, blockchain, viewStates, config, logger)
	case leaderrotation.ModuleNameReputation:
		ld = leaderrotation.NewRepBased(chainLength, viewStates, config, logger)
	default:
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", name)
	}
	return
}

func NewCommunicationModule(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	sender core.Sender,
	leaderRotation leaderrotation.LeaderRotation,
	viewStates *protocol.ViewStates,
	name string,
) (comm comm.Communication, _ error) {
	logger.Debugf("initializing module (propagation): %s", name)
	switch name {
	case kauri.ModuleName:
		comm = kauri.New(
			logger,
			eventLoop,
			config,
			blockchain,
			auth,
			kauri.WrapGorumsSender(
				eventLoop,
				config,
				sender.(*network.GorumsSender), // TODO(AlanRostem): avoid cast
			),
		)
	case clique.ModuleName:
		comm = clique.New(
			config,
			votingmachine.New(
				logger,
				eventLoop,
				config,
				blockchain,
				auth,
				viewStates,
			),
			leaderRotation,
			sender,
		)
	default:
		return nil, fmt.Errorf("invalid propagation scheme: '%s'", name)
	}
	return
}
