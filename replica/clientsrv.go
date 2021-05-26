package replica

import (
	"context"
	"io"
	"log"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"google.golang.org/protobuf/proto"
)

// clientSrv serves a client.
type clientSrv struct {
	mut      sync.Mutex
	mod      *consensus.Modules
	srv      *gorums.Server
	handlers map[cmdID]func(*empty.Empty, error)
	cmdCache *cmdCache
	output   io.Writer
}

// newClientServer returns a new client server.
func newClientServer(conf *orchestrationpb.ReplicaRunConfig, srvOpts []gorums.ServerOption) (srv *clientSrv, err error) {
	srv = &clientSrv{
		handlers: make(map[cmdID]func(*empty.Empty, error)),
		srv:      gorums.NewServer(srvOpts...),
		cmdCache: newCmdCache(int(conf.GetBatchSize())),
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
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.mod.Logger().Error(err)
		}
	}()
	return nil
}

func (srv *clientSrv) Stop() {
	srv.srv.Stop()
}

func (srv *clientSrv) ExecCommand(_ context.Context, cmd *clientpb.Command, out func(*empty.Empty, error)) {
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}

	srv.mut.Lock()
	srv.handlers[id] = out
	srv.mut.Unlock()

	srv.cmdCache.addCommand(cmd)
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
		_, err := srv.output.Write(cmd.Data)
		if err != nil {
			log.Printf("Error writing data: %v\n", err)
		}
		srv.mut.Lock()
		if reply, ok := srv.handlers[cmdID{cmd.ClientID, cmd.SequenceNumber}]; ok {
			reply(&empty.Empty{}, nil)
		}
		srv.mut.Unlock()
	}
}
