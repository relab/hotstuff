package server

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ClientIO serves a client.
type ClientIO struct {
	logger   logging.Logger
	cmdCache *clientpb.CommandCache

	mut          sync.Mutex
	srv          *gorums.Server
	awaitingCmds map[clientpb.MessageID]chan<- error
	hash         hash.Hash
	cmdCount     uint32

	lastExecutedSeqNum map[uint32]uint64 // highest executed sequence number per client ID
}

// NewClientIO returns a new client IO server.
func NewClientIO(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	cmdCache *clientpb.CommandCache,
	srvOpts ...gorums.ServerOption,
) (srv *ClientIO) {
	srv = &ClientIO{
		logger:   logger,
		cmdCache: cmdCache,

		awaitingCmds:       make(map[clientpb.MessageID]chan<- error),
		srv:                gorums.NewServer(srvOpts...),
		hash:               sha256.New(),
		lastExecutedSeqNum: make(map[uint32]uint64),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	eventLoop.RegisterHandler(clientpb.ExecuteEvent{}, func(event any) {
		e := event.(clientpb.ExecuteEvent)
		srv.Exec(e.Batch)
	})
	eventLoop.RegisterHandler(clientpb.AbortEvent{}, func(event any) {
		e := event.(clientpb.AbortEvent)
		srv.Abort(e.Batch)
	})
	return srv
}

func (srv *ClientIO) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

func (srv *ClientIO) Stop() {
	srv.srv.Stop()
}

func (srv *ClientIO) Hash() hash.Hash {
	return srv.hash
}

func (srv *ClientIO) CmdCount() uint32 {
	return srv.cmdCount
}

func (srv *ClientIO) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*emptypb.Empty, error) {
	id := cmd.ID()
	errChan := make(chan error)

	srv.mut.Lock()
	srv.awaitingCmds[id] = errChan
	srv.mut.Unlock()

	srv.cmdCache.Add(cmd)
	ctx.Release()
	err := <-errChan
	return &emptypb.Empty{}, err
}

func (srv *ClientIO) Exec(batch *clientpb.Batch) {
	for _, cmd := range batch.GetCommands() {
		id := cmd.ID()

		srv.mut.Lock()
		if srv.isDuplicate(cmd) {
			srv.logger.Info("duplicate command found")
			srv.completeCommand(id, status.Error(codes.Aborted, "command already executed"))
			srv.mut.Unlock()
			continue
		}
		srv.lastExecutedSeqNum[cmd.ClientID] = cmd.SequenceNumber
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		srv.completeCommand(id, nil)
		srv.mut.Unlock()
	}
	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *ClientIO) Abort(batch *clientpb.Batch) {
	for _, cmd := range batch.GetCommands() {
		srv.mut.Lock()
		srv.completeCommand(cmd.ID(), status.Error(codes.Aborted, "blockchain was forked"))
		srv.mut.Unlock()
	}
}

// isDuplicate return true if the command has already been executed.
// The caller must hold srv.mut.Lock().
func (srv *ClientIO) isDuplicate(cmd *clientpb.Command) bool {
	seqNum, ok := srv.lastExecutedSeqNum[cmd.ClientID]
	return ok && seqNum >= cmd.SequenceNumber
}

// completeCommand sends an error or nil to the awaiting client's error channel.
// The caller must hold srv.mut.Lock().
func (srv *ClientIO) completeCommand(id clientpb.MessageID, err error) {
	if errChan, ok := srv.awaitingCmds[id]; ok {
		errChan <- err
		delete(srv.awaitingCmds, id)
	}
}
