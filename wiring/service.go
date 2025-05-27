package wiring

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/service/clientsrv"
	"github.com/relab/hotstuff/service/committer"
)

type Service struct {
	cmdCache  *clientpb.Cache
	clientSrv *clientsrv.Server
	committer *committer.Committer
}

// NewService returns a set of dependencies managing application service, such as serving clients through the
// network and committing and executing requests.
func NewService(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	blockChain *blockchain.BlockChain,
	rules modules.HotstuffRuleset,
	// TODO: Join these into single option type
	cacheOpt []clientpb.Option,
	clientSrvOpts ...gorums.ServerOption,
) *Service {
	cmdCache := clientpb.New(
		cacheOpt...,
	)
	clientSrv := clientsrv.New(
		eventLoop,
		logger,
		cmdCache,
		clientSrvOpts...,
	)
	return &Service{
		cmdCache:  cmdCache,
		clientSrv: clientSrv,
		committer: committer.New(
			eventLoop,
			logger,
			blockChain,
			rules,
			clientSrv,
		),
	}
}

// CmdCache returns the command cache.
func (s *Service) CmdCache() *clientpb.Cache {
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
