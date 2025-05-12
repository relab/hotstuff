package dependencies

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/committer"
)

type Service struct {
	cmdCache  *cmdcache.Cache
	clientSrv *clientsrv.Server
	committer *committer.Committer
}

func NewService(
	depsCore *Core,
	depsSecure *Security,
	cacheOpt []cmdcache.Option,
	clientSrvOpts ...gorums.ServerOption,
) *Service {
	cmdCache := cmdcache.New(
		depsCore.Logger(),
		cacheOpt...,
	)
	clientSrv := clientsrv.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		cmdCache,
		clientSrvOpts...,
	)
	return &Service{
		cmdCache:  cmdCache,
		clientSrv: clientSrv,
		committer: committer.New(
			depsSecure.BlockChain(),
			clientSrv,
			depsCore.Logger(),
		),
	}
}

// GetCmdCache returns the command cache.
func (s *Service) CmdCache() *cmdcache.Cache {
	return s.cmdCache
}

// GetClientSrv returns the client server.
func (s *Service) ClientSrv() *clientsrv.Server {
	return s.clientSrv
}

// GetCommitter returns the committer.
func (s *Service) Committer() *committer.Committer {
	return s.committer
}
