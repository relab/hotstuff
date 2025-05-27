package clientsrv

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/hotstuff"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server serves a client.
type Server struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	cmdCache  *clientpb.Cache

	mut          sync.Mutex
	srv          *gorums.Server
	awaitingCmds map[clientpb.MessageID]chan<- error
	hash         hash.Hash
	cmdCount     uint32
}

// New returns a new client server.
func New(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	cmdCache *clientpb.Cache,

	srvOpts ...gorums.ServerOption,
) (srv *Server) {
	srv = &Server{
		eventLoop: eventLoop,
		logger:    logger,
		cmdCache:  cmdCache,

		awaitingCmds: make(map[clientpb.MessageID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		hash:         sha256.New(),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	return srv
}

func (srv *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv.StartOnListener(lis)
	return nil
}

func (srv *Server) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

func (srv *Server) Stop() {
	srv.srv.Stop()
}

func (srv *Server) CmdCache() *clientpb.Cache {
	return srv.cmdCache
}

func (srv *Server) Hash() hash.Hash {
	return srv.hash
}

func (srv *Server) CmdCount() uint32 {
	return srv.cmdCount
}

func (srv *Server) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*emptypb.Empty, error) {
	id := cmd.ID()
	c := make(chan error)

	srv.mut.Lock()
	srv.awaitingCmds[id] = c
	srv.mut.Unlock()

	srv.cmdCache.Add(cmd)
	ctx.Release()
	err := <-c
	return &emptypb.Empty{}, err
}

func (srv *Server) Exec(cmd hotstuff.Command) {
	batch, err := srv.cmdCache.GetCommands(cmd)
	if err != nil {
		srv.logger.Error(err)
		return
	}

	srv.eventLoop.AddEvent(hotstuff.CommitEvent{Commands: len(batch)})

	for _, cmd := range batch {
		id := cmd.ID()

		srv.mut.Lock()
		// TODO(meling): ASKING: previously the hash.Write and srv.cmdCount++ were outside the critical section, shouldn't they be inside?
		// TODO(meling): We should add a concurrency test for this logic to check that the hash doesn't get corrupted.
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- nil
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}

	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *Server) Fork(cmd hotstuff.Command) {
	batch, err := srv.cmdCache.GetCommands(cmd)
	if err != nil {
		srv.logger.Error(err)
		return
	}
	for _, cmd := range batch {
		id := cmd.ID()

		srv.mut.Lock()
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- status.Error(codes.Aborted, "blockchain was forked")
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}
}
