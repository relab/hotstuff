package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/committer"
)

type Service struct {
	CmdCache  *clientsrv.CmdCache
	ClientSrv *clientsrv.ClientServer
	Committer *committer.Committer
}

func NewService(
	depsCore *Core,
	depsSecure *Security,
	cacheOpt []clientsrv.CacheOption,
	clientSrvOpts ...gorums.ServerOption,
) *Service {
	cmdCache := clientsrv.NewCmdCache(
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
