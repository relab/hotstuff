package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
	"google.golang.org/grpc/credentials"
)

type Network struct {
	Config *netconfig.Config
	Sender *sender.Sender
}

func NewNetwork(
	depsCore *Core,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *Network {
	cfg := netconfig.NewConfig()
	return &Network{
		Config: cfg,
		Sender: sender.New(
			cfg,
			depsCore.EventLoop,
			depsCore.Logger,
			depsCore.Globals,
			creds,
			mgrOpts...,
		),
	}
}
