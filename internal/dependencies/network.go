package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
	"google.golang.org/grpc/credentials"
)

type DepSetNetwork struct {
	Config *netconfig.Config
	Sender *sender.Sender
}

func NewNetwork(
	depsCore *DepSetCore,
	creds credentials.TransportCredentials,
	mgrOpts ...gorums.ManagerOption,
) *DepSetNetwork {
	cfg := netconfig.NewConfig()
	send := sender.New(
		cfg,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
		creds,
		mgrOpts...,
	)

	return &DepSetNetwork{
		Config: cfg,
		Sender: send,
	}
}
