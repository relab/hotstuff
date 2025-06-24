package wiring

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/comm/clique"
	"github.com/relab/hotstuff/protocol/comm/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

// TODO(AlanRostem): make a "comm" package and move this method into it
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
