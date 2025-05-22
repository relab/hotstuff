package consensus

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type Option func(*Consensus)

// WithKauri constructs the Kauri protocol and overrides dissemination and aggregation methods.
func WithKauri(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
	sender *network.GorumsSender,
) Option {
	return func(cs *Consensus) {
		k := kauri.New(
			logger,
			eventLoop,
			config,
			blockChain,
			auth,
			sender,
		)
		cs.extDisseminator = k
		cs.extHandler = k
	}
}
