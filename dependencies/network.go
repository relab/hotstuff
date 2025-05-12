package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/network/sender"
	"google.golang.org/grpc/credentials"
)

type Network struct {
	sender *sender.Sender
}

func NewNetwork(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	globals *globals.Globals,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *Network {
	return &Network{
		sender: sender.New(
			eventLoop,
			logger,
			globals,
			creds,
			mgrOpts...,
		),
	}
}

// Sender returns the sender instance.
func (n *Network) Sender() *sender.Sender {
	return n.sender
}
