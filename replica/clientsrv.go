package replica

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// clientSrv serves a client.
type clientSrv struct {
	mut          sync.Mutex
	mods         *modules.Modules
	srv          *gorums.Server
	awaitingCmds map[cmdID]chan<- error
	cmdCache     *cmdCache
	hash         hash.Hash
}

// newClientServer returns a new client server.
func newClientServer(conf Config, srvOpts []gorums.ServerOption) (srv *clientSrv) {
	srv = &clientSrv{
		awaitingCmds: make(map[cmdID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		cmdCache:     newCmdCache(int(conf.BatchSize)),
		hash:         sha256.New(),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	return srv
}

// InitModule gives the module access to the other modules.
func (srv *clientSrv) InitModule(mods *modules.Modules) {
	srv.mods = mods
	srv.cmdCache.InitModule(mods)
}

func (srv *clientSrv) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv.StartOnListener(lis)
	return nil
}

func (srv *clientSrv) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.mods.Logger().Error(err)
		}
	}()
}

func (srv *clientSrv) Stop() {
	srv.srv.Stop()
}

func (srv *clientSrv) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*emptypb.Empty, error) {
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}

	c := make(chan error)
	srv.mut.Lock()
	srv.awaitingCmds[id] = c
	srv.mut.Unlock()

	srv.cmdCache.addCommand(cmd)
	ctx.Release()
	err := <-c
	return &emptypb.Empty{}, err
}

func (srv *clientSrv) Exec(cmd consensus.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.mods.Logger().Errorf("Failed to unmarshal command: %v", err)
		return
	}

	srv.mods.EventLoop().AddEvent(consensus.CommitEvent{Commands: len(batch.GetCommands())})

	for _, cmd := range batch.GetCommands() {
		_, _ = srv.hash.Write(cmd.Data)
		srv.mut.Lock()
		id := cmdID{cmd.GetClientID(), cmd.GetSequenceNumber()}
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- nil
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}

	srv.mods.Logger().Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *clientSrv) Fork(cmd consensus.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.mods.Logger().Errorf("Failed to unmarshal command: %v", err)
		return
	}

	for _, cmd := range batch.GetCommands() {
		srv.mut.Lock()
		id := cmdID{cmd.GetClientID(), cmd.GetSequenceNumber()}
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- status.Error(codes.Aborted, "blockchain was forked")
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}
}
