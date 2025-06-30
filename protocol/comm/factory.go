package comm

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

func New(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	sender core.Sender,
	leaderRotation leaderrotation.LeaderRotation,
	viewStates *protocol.ViewStates,
	name string,
) (communication Communication, _ error) {
	switch name {
	case NameKauri:
		communication = NewKauri(
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
	case NameClique:
		communication = NewClique(
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
		return nil, fmt.Errorf("invalid communication type: '%s'", name)
	}
	return
}
