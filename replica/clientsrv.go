package replica

import (
	"crypto/sha256"
	"hash"
	"log"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/protobuf/proto"
)

// clientSrv serves a client.
type clientSrv struct {
	mut          sync.Mutex
	mod          *consensus.Modules
	srv          *gorums.Server
	awaitingCmds map[cmdID]chan<- struct{}
	cmdCache     *cmdCache
	dataLogger   consensus.DataLogger
	hash         hash.Hash
}

// newClientServer returns a new client server.
func newClientServer(conf Config, srvOpts []gorums.ServerOption) (srv *clientSrv, err error) {
	srv = &clientSrv{
		awaitingCmds: make(map[cmdID]chan<- struct{}),
		srv:          gorums.NewServer(srvOpts...),
		cmdCache:     newCmdCache(int(conf.BatchSize)),
		dataLogger:   conf.DataLogger,
		hash:         sha256.New(),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	return srv, nil
}

// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
// settings using the ConfigBuilder.
func (srv *clientSrv) InitModule(mod *consensus.Modules, opts *consensus.OptionsBuilder) {
	srv.mod = mod
	srv.cmdCache.InitModule(mod, opts)
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
			srv.mod.Logger().Error(err)
		}
	}()
}

func (srv *clientSrv) Stop() {
	srv.srv.Stop()
}

func (srv *clientSrv) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*empty.Empty, error) {
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}

	c := make(chan struct{})
	srv.mut.Lock()
	srv.awaitingCmds[id] = c
	srv.mut.Unlock()

	srv.cmdCache.addCommand(cmd)
	ctx.Release()
	<-c
	return &empty.Empty{}, nil
}

func (srv *clientSrv) Exec(cmd consensus.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		log.Printf("Failed to unmarshal command: %v\n", err)
		return
	}

	// TODO: add latency measurements

	for _, cmd := range batch.GetCommands() {
		_, _ = srv.hash.Write(cmd.Data)
		if err != nil {
			log.Printf("Error writing data: %v\n", err)
		}
		srv.mut.Lock()
		if done, ok := srv.awaitingCmds[cmdID{cmd.ClientID, cmd.SequenceNumber}]; ok {
			close(done)
		}
		srv.mut.Unlock()
	}
}
