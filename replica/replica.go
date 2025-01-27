// Package replica provides the required code for starting and running a replica and handling client requests.
package replica

import (
	"context"
	"net"

	"github.com/relab/hotstuff/clientsrv"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/sender"
	"github.com/relab/hotstuff/synchronizer"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/server"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Replica is a participant in the consensus protocol.
type Replica struct {
	clientSrv    *clientsrv.ClientServer
	cfg          *netconfig.Config
	hsSrv        *server.Server
	sender       *sender.Sender
	eventLoop    *eventloop.EventLoop
	synchronizer *synchronizer.Synchronizer

	execHandlers map[clientsrv.CmdID]func(*emptypb.Empty, error)
	cancel       context.CancelFunc
	done         chan struct{}
}

// New returns a new replica.
func New(
	clientSrv *clientsrv.ClientServer,
	server *server.Server,
	sender *sender.Sender,
	eventLoop *eventloop.EventLoop,
	synchronizer *synchronizer.Synchronizer,
) (replica *Replica) {
	srv := &Replica{
		clientSrv:    clientSrv,
		sender:       sender,
		hsSrv:        server,
		eventLoop:    eventLoop,
		synchronizer: synchronizer,

		execHandlers: make(map[clientsrv.CmdID]func(*emptypb.Empty, error)),
		cancel:       func() {},
		done:         make(chan struct{}),
	}

	return srv
}

// StartServers starts the client and replica servers.
func (srv *Replica) StartServers(replicaListen, clientListen net.Listener) {
	srv.hsSrv.StartOnListener(replicaListen)
	srv.clientSrv.StartOnListener(clientListen)
}

// Connect connects to the other replicas.
func (srv *Replica) Connect(replicas []hotstuff.ReplicaInfo) error {
	return srv.sender.Connect(replicas)
}

// Start runs the replica in a goroutine.
func (srv *Replica) Start() {
	var ctx context.Context
	ctx, srv.cancel = context.WithCancel(context.Background())
	go func() {
		srv.Run(ctx)
		close(srv.done)
	}()
}

// Stop stops the replica and closes connections.
func (srv *Replica) Stop() {
	srv.cancel()
	<-srv.done
	srv.Close()
}

// Run runs the replica until the context is canceled.
func (srv *Replica) Run(ctx context.Context) {
	srv.synchronizer.Start(ctx)
	srv.eventLoop.Run(ctx)
}

// Close closes the connections and stops the servers used by the replica.
func (srv *Replica) Close() {
	srv.clientSrv.Stop()
	srv.sender.Close()
	srv.hsSrv.Stop()
}

// GetHash returns the hash of all executed commands.
func (srv *Replica) GetHash() (b []byte) {
	return srv.clientSrv.Hash().Sum(b)
}

// GetCmdCount returns the count of all executed commands.
func (srv *Replica) GetCmdCount() (c uint32) {
	return srv.clientSrv.CmdCount()
}
