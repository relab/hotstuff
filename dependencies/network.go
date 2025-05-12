package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
	"google.golang.org/grpc/credentials"
)

type Network struct {
	config *netconfig.Config
	sender *sender.Sender
}

func NewNetwork(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	globals *globals.Globals,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *Network {
	cfg := netconfig.NewConfig()
	return &Network{
		config: cfg,
		sender: sender.New(
			cfg,
			eventLoop,
			logger,
			globals,
			creds,
			mgrOpts...,
		),
	}
}

// Config returns the network configuration.
func (n *Network) Config() *netconfig.Config {
	return n.config
}

// Sender returns the sender instance.
func (n *Network) Sender() *sender.Sender {
	return n.sender
}
