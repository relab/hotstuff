package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network"
	"google.golang.org/grpc/credentials"
)

// TODO(AlanRostem): consider removing this
type Network struct {
	sender *network.Sender
}

func NewNetwork(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	globals *globals.Globals,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *Network {
	return &Network{
		sender: network.NewSender(
			eventLoop,
			logger,
			globals,
			creds,
			mgrOpts...,
		),
	}
}

// Sender returns the sender instance.
func (n *Network) Sender() *network.Sender {
	return n.sender
}
