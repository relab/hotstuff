package wiring

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/server"
)

type Client struct {
	cmdCache *clientpb.CommandCache
	clientIO *server.ClientIO
}

// NewClient returns a set of dependencies for serving clients through
func NewClient(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	commandBatchSize uint32,
	clientSrvOpts ...gorums.ServerOption,
) *Client {
	cmdCache := clientpb.NewCommandCache(
		commandBatchSize,
	)
	clientSrv := server.NewClientIO(
		eventLoop,
		logger,
		cmdCache,
		clientSrvOpts...,
	)
	return &Client{
		cmdCache: cmdCache,
		clientIO: clientSrv,
	}
}

// Cache returns the command cache.
func (s *Client) Cache() *clientpb.CommandCache {
	return s.cmdCache
}

// Server returns the client server.
func (s *Client) Server() *server.ClientIO {
	return s.clientIO
}
