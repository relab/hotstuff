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
	"github.com/relab/hotstuff/service/cmdcache"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ClientServer serves a client.
type ClientServer struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	cmdCache  *cmdcache.Cache

	mut          sync.Mutex
	srv          *gorums.Server
	awaitingCmds map[cmdcache.CmdID]chan<- error
	hash         hash.Hash
	cmdCount     uint32
}

// NewClientServer returns a new client server.
func NewClientServer(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	cmdCache *cmdcache.Cache,

	srvOpts ...gorums.ServerOption,
) (srv *ClientServer) {
	srv = &ClientServer{
		eventLoop: eventLoop,
		logger:    logger,
		cmdCache:  cmdCache,

		awaitingCmds: make(map[cmdcache.CmdID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		hash:         sha256.New(),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	return srv
}

func (srv *ClientServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv.StartOnListener(lis)
	return nil
}

func (srv *ClientServer) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

func (srv *ClientServer) Stop() {
	srv.srv.Stop()
}

func (srv *ClientServer) CmdCache() *cmdcache.Cache {
	return srv.cmdCache
}

func (srv *ClientServer) Hash() hash.Hash {
	return srv.hash
}

func (srv *ClientServer) CmdCount() uint32 {
	return srv.cmdCount
}

func (srv *ClientServer) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*emptypb.Empty, error) {
	id := cmdcache.CmdID{
		ClientID:    cmd.ClientID,
		SequenceNum: cmd.SequenceNumber,
	}

	c := make(chan error)
	srv.mut.Lock()
	srv.awaitingCmds[id] = c
	srv.mut.Unlock()

	srv.cmdCache.Add(cmd)
	ctx.Release()
	err := <-c
	return &emptypb.Empty{}, err
}

func (srv *ClientServer) Exec(cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.logger.Errorf("Failed to unmarshal command: %v", err)
		return
	}

	srv.eventLoop.AddEvent(hotstuff.CommitEvent{Commands: len(batch.GetCommands())})

	for _, cmd := range batch.GetCommands() {
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		srv.mut.Lock()
		id := cmdcache.CmdID{
			ClientID:    cmd.GetClientID(),
			SequenceNum: cmd.GetSequenceNumber(),
		}
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- nil
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}

	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *ClientServer) Fork(cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.logger.Errorf("Failed to unmarshal command: %v", err)
		return
	}

	for _, cmd := range batch.GetCommands() {
		srv.mut.Lock()
		id := cmdcache.CmdID{
			ClientID:    cmd.GetClientID(),
			SequenceNum: cmd.GetSequenceNumber(),
		}
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- status.Error(codes.Aborted, "blockchain was forked")
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}
}
