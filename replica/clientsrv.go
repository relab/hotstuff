package replica

import (
	"crypto/sha256"
	"hash"
	"hash/fnv"
	"net"
	"sync"

	"github.com/relab/hotstuff"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// clientSrv serves a client.
type clientSrv struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger

	mut             sync.Mutex
	srv             *gorums.Server
	awaitingCmds    map[cmdID]chan<- error
	cmdCaches       map[pipeline.Pipe]*cmdCache
	hash            hash.Hash
	cmdCount        uint32
	pipeCount       int
	cmdsSentToPipe  map[pipeline.Pipe]int
	cmdAddMethodStr string
}

// newClientServer returns a new client server.
func newClientServer(cmdCaches map[pipeline.Pipe]*cmdCache, cmdAddMethodStr string, srvOpts []gorums.ServerOption) (srv *clientSrv) {
	srv = &clientSrv{
		awaitingCmds: make(map[cmdID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		// cmdCache:     newCmdCache(int(conf.BatchSize)),
		cmdCaches:       cmdCaches,
		pipeCount:       len(cmdCaches),
		hash:            sha256.New(),
		cmdsSentToPipe:  make(map[pipeline.Pipe]int),
		cmdAddMethodStr: cmdAddMethodStr,
	}

	clientpb.RegisterClientServer(srv.srv, srv)
	return srv
}

func (srv *clientSrv) addCommandToSmallestCache(cmd *clientpb.Command) {
	smallestCachePipe := pipeline.NullPipe
	smallestCacheCount := 0
	for pipe := range srv.cmdCaches {
		count := srv.cmdCaches[pipe].commandCount()
		if smallestCacheCount > count || smallestCachePipe == pipeline.NullPipe {
			smallestCachePipe = pipe
			smallestCacheCount = count
		}
	}
	srv.cmdCaches[smallestCachePipe].addCommand(cmd)
	srv.mut.Lock()
	srv.cmdsSentToPipe[smallestCachePipe]++
	srv.mut.Unlock()
}

func (srv *clientSrv) addCommandHashed(cmd *clientpb.Command) error {
	if srv.pipeCount > 1 {
		asBytes, err := proto.Marshal(cmd)
		if err != nil {
			return err
		}
		h := fnv.New32a()
		h.Write(asBytes)
		hSum := h.Sum32()

		correctPipe := pipeline.Pipe((hSum % uint32(srv.pipeCount)) + 1)
		cache, ok := srv.cmdCaches[correctPipe]
		if ok {
			cache.addCommand(cmd)
			srv.mut.Lock()
			srv.cmdsSentToPipe[correctPipe]++
			srv.mut.Unlock()
		} else {
			srv.logger.DPanicf("addCommand: pipe not found: %d. count was %d", correctPipe, srv.pipeCount)
		}
	} else {
		i := 0
		for _, cmdCache := range srv.cmdCaches {
			cmdCache.addCommand(cmd)
			i++
		}
		if i > 1 {
			panic("too many caches")
		}
	}

	return nil
}

func (srv *clientSrv) commandAddingMethod(cmd *clientpb.Command) {
	switch srv.cmdAddMethodStr {
	case "hashed":
		srv.addCommandHashed(cmd)
		break
	case "smallest":
		srv.addCommandToSmallestCache(cmd)
		break
	}
}

// InitModule gives the module access to the other modules.
func (srv *clientSrv) InitModule(mods *modules.Core, _ modules.InitOptions) {
	mods.Get(
		&srv.eventLoop,
		&srv.logger,
	)
	// srv.cmdCache.InitModule(mods, buildOpt)
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
			srv.logger.Error(err)
		}
	}()
}

func (srv *clientSrv) Stop() {
	srv.srv.Stop()
}

func (srv *clientSrv) PrintCmdResult() {
	srv.logger.Info("Command count per pipe results:")
	for pipe, count := range srv.cmdsSentToPipe {
		srv.logger.Infof("\tP%d=(%d)", pipe, count)
	}
}

func (srv *clientSrv) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) (*emptypb.Empty, error) {
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}

	c := make(chan error)
	srv.mut.Lock()
	srv.awaitingCmds[id] = c
	srv.mut.Unlock()

	srv.commandAddingMethod(cmd)
	ctx.Release()
	err := <-c
	return &emptypb.Empty{}, err
}

func (srv *clientSrv) Exec(cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.logger.Errorf("Failed to unmarshal command: %v", err)
		return
	}

	srv.eventLoop.AddEvent(hotstuff.CommitEvent{Commands: len(batch.GetCommands())})
	srv.logger.Debugf("Executed %d commands", len(batch.GetCommands()))

	for _, cmd := range batch.GetCommands() {
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		srv.mut.Lock()
		id := cmdID{cmd.GetClientID(), cmd.GetSequenceNumber()}
		if done, ok := srv.awaitingCmds[id]; ok {
			done <- nil
			delete(srv.awaitingCmds, id)
		}
		srv.mut.Unlock()
	}

	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *clientSrv) Fork(cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.logger.Errorf("Failed to unmarshal command: %v", err)
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
