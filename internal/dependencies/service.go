package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/committer"
)

type Service struct {
	CmdCache  *cmdcache.Cache
	ClientSrv *clientsrv.ClientServer
	Committer *committer.Committer
}

func NewService(
	depsCore *Core,
	depsSecure *Security,
	cacheOpt []cmdcache.Option,
	clientSrvOpts ...gorums.ServerOption,
) *Service {
	cmdCache := cmdcache.New(
		depsCore.Logger,
		cacheOpt...,
	)
	clientSrv := clientsrv.NewClientServer(
		depsCore.EventLoop,
		depsCore.Logger,
		cmdCache,
		clientSrvOpts...,
	)
	return &Service{
		CmdCache:  cmdCache,
		ClientSrv: clientSrv,
		Committer: committer.New(
			depsSecure.BlockChain,
			clientSrv,
			depsCore.Logger,
		),
	}
}
