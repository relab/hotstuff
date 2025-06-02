package wiring

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

type Client struct {
	cmdCache  *clientpb.Cache
	clientSrv *clientpb.Server
}

// NewClient returns a set of dependencies for serving clients through
func NewClient(
	logger logging.Logger,
	// TODO: Join these into single option type
	cacheOpt []clientpb.Option,
	clientSrvOpts ...gorums.ServerOption,
) *Client {
	cmdCache := clientpb.New(
		cacheOpt...,
	)
	clientSrv := clientpb.NewServer(
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
func (s *Client) Cache() *clientpb.Cache {
	return s.cmdCache
}

// Server returns the client server.
func (s *Client) Server() *clientpb.Server {
	return s.clientSrv
}
