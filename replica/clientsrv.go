package replica

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/hotstuff"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// clientSrv serves a client.
type clientSrv struct {
	eventLoop *eventloop.ScopedEventLoop
	logger    logging.Logger

	mut                sync.Mutex
	srv                *gorums.Server
	awaitingCmds       map[cmdID]chan<- error
	cmdCaches          map[hotstuff.Instance]*cmdCache
	hash               hash.Hash
	cmdCount           uint32
	instanceCount      int
	cmdsSentToInstance map[hotstuff.Instance]int
	cmdAddMethodStr    string
	marshaler          proto.MarshalOptions
}

// newClientServer returns a new client server.
func newClientServer(cmdAddMethodStr string, srvOpts []gorums.ServerOption) (srv *clientSrv) {
	srv = &clientSrv{
		awaitingCmds: make(map[cmdID]chan<- error),
		srv:          gorums.NewServer(srvOpts...),
		// cmdCache:     newCmdCache(int(conf.BatchSize)),
		cmdCaches:          make(map[hotstuff.Instance]*cmdCache),
		hash:               sha256.New(),
		cmdsSentToInstance: make(map[hotstuff.Instance]int),
		cmdAddMethodStr:    cmdAddMethodStr,
		marshaler:          proto.MarshalOptions{Deterministic: true},
	}

	clientpb.RegisterClientServer(srv.srv, srv)
	return srv
}

func (srv *clientSrv) addCommandToSmallestCache(cmd *clientpb.Command) {
	smallestCacheInstance := hotstuff.ZeroInstance
	smallestCacheCount := 0
	for instance := range srv.cmdCaches {
		count := srv.cmdCaches[instance].commandCount()
		if smallestCacheCount > count || smallestCacheInstance == hotstuff.ZeroInstance {
			smallestCacheInstance = instance
			smallestCacheCount = count
		}
	}
	srv.cmdCaches[smallestCacheInstance].addCommand(cmd)
	srv.mut.Lock()
	srv.cmdsSentToInstance[smallestCacheInstance]++
	srv.mut.Unlock()
}

func (srv *clientSrv) addCommandHashed(cmd *clientpb.Command) error {
	correctInstance := hotstuff.Instance((uint32(cmd.SequenceNumber) % uint32(srv.instanceCount)) + 1)
	cache, ok := srv.cmdCaches[correctInstance]
	if ok {
		cache.addCommand(cmd)
		srv.mut.Lock()
		srv.cmdsSentToInstance[correctInstance]++
		srv.mut.Unlock()
	} else {
		srv.logger.DPanicf("addCommand: instance not found: %d. count was %d", correctInstance, srv.instanceCount)
	}
	return nil
}

func (srv *clientSrv) commandAddingMethod(cmd *clientpb.Command) {
	if srv.instanceCount == 0 {
		srv.cmdCaches[hotstuff.ZeroInstance].addCommand(cmd)
		return
	}

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
func (srv *clientSrv) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	mods.Get(
		&srv.eventLoop,
		&srv.logger,
	)

	srv.instanceCount = info.ScopeCount
	if info.IsPipeliningEnabled {
		for _, scope := range mods.Scopes() {
			var cache *cmdCache
			mods.MatchForScope(scope, &cache)
			srv.cmdCaches[scope] = cache
		}
		return
	}

	var cache *cmdCache
	mods.Get(&cache)
	srv.cmdCaches[hotstuff.ZeroInstance] = cache

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

func (srv *clientSrv) PrintScopedCmdResult() {
	if srv.instanceCount <= 1 {
		return
	}
	srv.logger.Info("Command count per instance results:")
	for instance, count := range srv.cmdsSentToInstance {
		srv.logger.Infof("\tP%d=(%d)", instance, count)
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

func (srv *clientSrv) Exec(onPipe hotstuff.Instance, cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		srv.logger.Errorf("Failed to unmarshal command: %v", err)
		return
	}

	srv.eventLoop.AddEvent(hotstuff.CommitEvent{Instance: onPipe, Commands: len(batch.GetCommands())})
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
