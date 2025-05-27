package wiring

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/service/clientsrv"
)

type Service struct {
	cmdCache  *clientpb.Cache
	clientSrv *clientsrv.Server
}

// NewClient returns a set of dependencies for serving clients through
func NewClient(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
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
	}
}

// Cache returns the command cache.
func (s *Service) Cache() *clientpb.Cache {
	return s.cmdCache
}

// Server returns the client server.
func (s *Service) Server() *clientsrv.Server {
	return s.clientSrv
}
