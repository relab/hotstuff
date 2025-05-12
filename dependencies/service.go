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

// NewService returns a set of dependencies managing application service, such as serving clients through the
// network and committing and executing requests.
func NewService(
	depsCore *Core,
	depsSecure *Security,
	cacheOpt []cmdcache.Option, // TODO: Join this into single option type
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

// CmdCache returns the command cache.
func (s *Service) CmdCache() *cmdcache.Cache {
	return s.cmdCache
}

// ClientSrv returns the client server.
func (s *Service) ClientSrv() *clientsrv.Server {
	return s.clientSrv
}

// Committer returns the committer.
func (s *Service) Committer() *committer.Committer {
	return s.committer
}
