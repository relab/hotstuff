package wiring

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

type Client struct {
	cmdCache  *clientpb.CommandCache
	clientSrv *clientpb.Server
}

// NewClient returns a set of dependencies for serving clients through
func NewClient(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	// TODO: Join these into single option type
	cacheOpt []clientpb.CommandCacheOption,
	clientSrvOpts ...gorums.ServerOption,
) *Client {
	cmdCache := clientpb.NewCommandCache(
		cacheOpt...,
	)
	clientSrv := clientpb.NewServer(
		eventLoop,
		logger,
		cmdCache,
		clientSrvOpts...,
	)
	return &Client{
		cmdCache:  cmdCache,
		clientSrv: clientSrv,
	}
}

// Cache returns the command cache.
func (s *Client) Cache() *clientpb.CommandCache {
	return s.cmdCache
}

// Server returns the client server.
func (s *Client) Server() *clientpb.Server {
	return s.clientSrv
}
