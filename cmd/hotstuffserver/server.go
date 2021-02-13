package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	hotstuffgorums "github.com/relab/hotstuff/backend/gorums"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/synchronizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

// cmdID is a unique identifier for a command
type cmdID struct {
	clientID    uint32
	sequenceNum uint64
}

type clientSrv struct {
	ctx       context.Context
	cancel    context.CancelFunc
	conf      *options
	gorumsSrv *gorums.Server
	hsSrv     *hotstuffgorums.Server
	cfg       *hotstuffgorums.Config
	hs        hotstuff.Consensus
	pm        hotstuff.ViewSynchronizer
	cmdCache  *cmdCache

	mut          sync.Mutex
	finishedCmds map[cmdID]chan struct{}

	lastExecTime int64
}

func newClientServer(conf *options, replicaConfig *config.ReplicaConfig, tlsCert *tls.Certificate) *clientSrv {
	ctx, cancel := context.WithCancel(context.Background())

	serverOpts := []gorums.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if conf.TLS {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(credentials.NewServerTLSFromCert(tlsCert)))
	}

	serverOpts = append(serverOpts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	srv := &clientSrv{
		ctx:          ctx,
		cancel:       cancel,
		conf:         conf,
		gorumsSrv:    gorums.NewServer(serverOpts...),
		cmdCache:     newCmdCache(conf.BatchSize),
		finishedCmds: make(map[cmdID]chan struct{}),
		lastExecTime: time.Now().UnixNano(),
	}

	var err error
	srv.cfg = hotstuffgorums.NewConfig(*replicaConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init gorums backend: %s\n", err)
		os.Exit(1)
	}

	srv.hsSrv = hotstuffgorums.NewServer(*replicaConfig)

	var leaderRotation hotstuff.LeaderRotation
	switch conf.PmType {
	case "fixed":
		leaderRotation = leaderrotation.NewFixed(conf.LeaderID)
	case "round-robin":
		leaderRotation = leaderrotation.NewRoundRobin(srv.cfg)
	default:
		fmt.Fprintf(os.Stderr, "Invalid pacemaker type: '%s'\n", conf.PmType)
		os.Exit(1)
	}
	srv.pm = synchronizer.New(leaderRotation, time.Duration(conf.ViewTimeout)*time.Millisecond)
	srv.hs = chainedhotstuff.Builder{
		Config:       srv.cfg,
		Acceptor:     srv.cmdCache,
		Executor:     srv,
		Synchronizer: srv.pm,
		CommandQueue: srv.cmdCache,
	}.Build()
	// Use a custom server instead of the gorums one
	client.RegisterClientServer(srv.gorumsSrv, srv)
	return srv
}

func (srv *clientSrv) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	err = srv.hsSrv.Start(srv.hs)
	if err != nil {
		return err
	}

	err = srv.cfg.Connect(10 * time.Second)
	if err != nil {
		return err
	}

	// sleep so that all replicas can be ready before we start
	time.Sleep(time.Duration(srv.conf.ViewTimeout) * time.Millisecond)

	srv.pm.Start()

	go func() {
		err := srv.gorumsSrv.Serve(lis)
		if err != nil {
			log.Println(err)
		}
	}()

	return nil
}

func (srv *clientSrv) Stop() {
	srv.pm.Stop()
	srv.cfg.Close()
	srv.hsSrv.Stop()
	srv.gorumsSrv.Stop()
	srv.cancel()
}

func (srv *clientSrv) ExecCommand(_ context.Context, cmd *client.Command, out func(*empty.Empty, error)) {
	finished := make(chan struct{})
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}
	srv.mut.Lock()
	srv.finishedCmds[id] = finished
	srv.mut.Unlock()

	srv.cmdCache.addCommand(cmd)

	go func(id cmdID, finished chan struct{}) {
		<-finished

		srv.mut.Lock()
		delete(srv.finishedCmds, id)
		srv.mut.Unlock()

		// send response
		out(&empty.Empty{}, nil)
	}(id, finished)
}

func (srv *clientSrv) Exec(cmd hotstuff.Command) {
	batch := new(client.Batch)
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal([]byte(cmd), batch)
	if err != nil {
		return
	}

	if len(batch.GetCommands()) > 0 && srv.conf.PrintThroughput {
		now := time.Now().UnixNano()
		prev := atomic.SwapInt64(&srv.lastExecTime, now)
		fmt.Printf("%d, %d\n", now-prev, len(batch.GetCommands()))
	}

	for _, cmd := range batch.GetCommands() {
		if err != nil {
			log.Printf("Failed to unmarshal command: %v\n", err)
		}
		if srv.conf.PrintCommands {
			fmt.Printf("%s", cmd.Data)
		}
		srv.mut.Lock()
		if c, ok := srv.finishedCmds[cmdID{cmd.ClientID, cmd.SequenceNumber}]; ok {
			c <- struct{}{}
		}
		srv.mut.Unlock()
	}
}
