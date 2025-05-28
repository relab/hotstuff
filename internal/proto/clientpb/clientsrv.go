package clientpb

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server serves a client.
type Server struct {
	logger   logging.Logger
	cmdCache *Cache

	mut          sync.Mutex
	srv          *gorums.Server
	awaitingCmds map[MessageID]chan<- error
	hash         hash.Hash
	cmdCount     uint32
}

// NewServer returns a new client server.
func NewServer(
	logger logging.Logger,
	cmdCache *Cache,
	srvOpts ...gorums.ServerOption,
) (srv *Server) {
	srv = &Server{
		logger:   logger,
		cmdCache: cmdCache,

		awaitingCmds: make(map[MessageID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		hash:         sha256.New(),
	}
	RegisterClientServer(srv.srv, srv)
	return srv
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

func (srv *Server) Hash() hash.Hash {
	return srv.hash
}

func (srv *Server) CmdCount() uint32 {
	return srv.cmdCount
}

func (srv *Server) ExecCommand(ctx gorums.ServerCtx, cmd *Command) (*emptypb.Empty, error) {
	id := cmd.ID()
	errChan := make(chan error)

	srv.mut.Lock()
	srv.awaitingCmds[id] = errChan
	srv.mut.Unlock()

	srv.cmdCache.add(cmd)
	ctx.Release()
	err := <-errChan
	return &emptypb.Empty{}, err
}

func (srv *Server) Exec(batch *Batch) {
	for _, cmd := range batch.GetCommands() {
		id := cmd.ID()

		srv.mut.Lock()
		// TODO(meling): ASKING: previously the hash.Write and srv.cmdCount++ were outside the critical section, shouldn't they be inside?
		// TODO(meling): We should add a concurrency test for this logic to check that the hash doesn't get corrupted.
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		if errChan, ok := srv.awaitingCmds[id]; ok {
			errChan <- nil
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}

	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *Server) Fork(batch *Batch) {
	for _, cmd := range batch.GetCommands() {
		id := cmd.ID()

		srv.mut.Lock()
		if errChan, ok := srv.awaitingCmds[id]; ok {
			errChan <- status.Error(codes.Aborted, "blockchain was forked")
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}
}
