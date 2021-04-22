package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
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
	"github.com/relab/hotstuff/consensus/fasthotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/leaderrotation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

func runServer(ctx context.Context, conf *options) {
	privkey, err := keygen.ReadPrivateKeyFile(conf.Privkey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key file: %v\n", err)
		os.Exit(1)
	}

	var creds credentials.TransportCredentials
	var tlsCert tls.Certificate
	if conf.TLS {
		creds, tlsCert = loadCreds(conf)
	}

	var clientAddress string
	replicaConfig := config.NewConfig(conf.SelfID, privkey, creds)
	for _, r := range conf.Replicas {
		key, err := keygen.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}

		info := &config.ReplicaInfo{
			ID:      r.ID,
			Address: r.PeerAddr,
			PubKey:  key,
		}

		if r.ID == conf.SelfID {
			// override own addresses if set
			if conf.ClientAddr != "" {
				clientAddress = conf.ClientAddr
			} else {
				clientAddress = r.ClientAddr
			}
			if conf.PeerAddr != "" {
				info.Address = conf.PeerAddr
			}
		}

		replicaConfig.Replicas[r.ID] = info
	}

	srv := newClientServer(conf, replicaConfig, &tlsCert)
	err = srv.Start(ctx, clientAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start HotStuff: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	srv.Stop()
}

func loadCreds(conf *options) (credentials.TransportCredentials, tls.Certificate) {
	if conf.Cert == "" {
		for _, replica := range conf.Replicas {
			if replica.ID == conf.SelfID {
				conf.Cert = replica.Cert
			}
		}
	}

	keyPath := conf.Privkey
	if conf.CertKey != "" {
		keyPath = conf.CertKey
	}

	tlsCert, err := tls.LoadX509KeyPair(conf.Cert, keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse certificate: %v\n", err)
		os.Exit(1)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		// system cert pool is unavailable on windows
		if runtime.GOOS != "windows" {
			fmt.Fprintf(os.Stderr, "Failed to get system cert pool: %v\n", err)
		}
		rootCAs = x509.NewCertPool()
	}

	for _, ca := range conf.RootCAs {
		cert, err := os.ReadFile(ca)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read CA file: %v\n", err)
			os.Exit(1)
		}
		if !rootCAs.AppendCertsFromPEM(cert) {
			fmt.Fprintf(os.Stderr, "Failed to add CA to cert pool.\n")
			os.Exit(1)
		}
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      rootCAs,
		ClientCAs:    rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})

	return creds, tlsCert
}

// cmdID is a unique identifier for a command
type cmdID struct {
	clientID    uint32
	sequenceNum uint64
}

type clientSrv struct {
	conf      *options
	output    io.Writer
	gorumsSrv *gorums.Server
	mgr       *hotstuffgorums.Manager
	hsSrv     *hotstuffgorums.Server
	hs        *hotstuff.HotStuff
	cmdCache  *cmdCache

	mut          sync.Mutex
	finishedCmds map[cmdID]chan struct{}

	lastExecTime int64
}

func newClientServer(conf *options, replicaConfig *config.ReplicaConfig, tlsCert *tls.Certificate) *clientSrv {
	serverOpts := []gorums.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if conf.TLS {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(credentials.NewServerTLSFromCert(tlsCert)))
	}

	serverOpts = append(serverOpts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	srv := &clientSrv{
		conf:         conf,
		gorumsSrv:    gorums.NewServer(serverOpts...),
		cmdCache:     newCmdCache(conf.BatchSize),
		finishedCmds: make(map[cmdID]chan struct{}),
		lastExecTime: time.Now().UnixNano(),
	}

	builder := chainedhotstuff.DefaultModules(
		*replicaConfig,
		hotstuff.ExponentialTimeout{BaseMS: conf.ViewTimeout, ExponentBase: 1, MaxExponent: 3},
	)
	srv.mgr = hotstuffgorums.NewManager(*replicaConfig)
	srv.hsSrv = hotstuffgorums.NewServer(*replicaConfig)
	builder.Register(srv.mgr, srv.hsSrv)

	var leaderRotation hotstuff.LeaderRotation
	switch conf.PmType {
	case "fixed":
		leaderRotation = leaderrotation.NewFixed(conf.LeaderID)
	case "round-robin":
		leaderRotation = leaderrotation.NewRoundRobin()
	default:
		fmt.Fprintf(os.Stderr, "Invalid pacemaker type: '%s'\n", conf.PmType)
		os.Exit(1)
	}
	var consensus hotstuff.Consensus
	switch conf.Consensus {
	case "chainedhotstuff":
		consensus = chainedhotstuff.New()
	case "fasthotstuff":
		consensus = fasthotstuff.New()
	default:
		fmt.Fprintf(os.Stderr, "Invalid consensus type: '%s'\n", conf.Consensus)
		os.Exit(1)
	}
	var cryptoImpl hotstuff.CryptoImpl
	switch conf.Crypto {
	case "ecdsa":
		cryptoImpl = ecdsa.New()
	case "bls12":
		cryptoImpl = bls12.New()
	default:
		fmt.Fprintf(os.Stderr, "Invalid crypto type: '%s'\n", conf.Crypto)
		os.Exit(1)
	}
	builder.Register(
		consensus,
		crypto.NewCache(cryptoImpl, 2*srv.mgr.Len()),
		leaderRotation,
		srv,          // executor
		srv.cmdCache, // acceptor and command queue
		logging.New(fmt.Sprintf("hs%d", conf.SelfID)),
	)
	srv.hs = builder.Build()

	// Use a custom server instead of the gorums one
	client.RegisterClientServer(srv.gorumsSrv, srv)
	return srv
}

func (srv *clientSrv) Start(ctx context.Context, address string) (err error) {
	if srv.conf.Output != "" {
		// Since io.Discard is not a WriteCloser, we just store the file as a Writer.
		srv.output, err = os.OpenFile(srv.conf.Output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	} else {
		// I wish we could make this a WriteCloser...
		srv.output = io.Discard
	}

	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	err = srv.hsSrv.Start()
	if err != nil {
		return err
	}

	err = srv.mgr.Connect(10 * time.Second)
	if err != nil {
		return err
	}

	// sleep so that all replicas can be ready before we start
	time.Sleep(time.Second)

	c := make(chan struct{})
	go func() {
		srv.hs.EventLoop().Run(ctx)
		close(c)
	}()

	go func() {
		err := srv.gorumsSrv.Serve(lis)
		if err != nil {
			log.Println(err)
		}
	}()

	// wait for the event loop to exit
	<-c
	return nil
}

func (srv *clientSrv) Stop() {
	srv.hs.ViewSynchronizer().Stop()
	srv.mgr.Close()
	srv.hsSrv.Stop()
	srv.gorumsSrv.Stop()
	if closer, ok := srv.output.(io.Closer); ok {
		err := closer.Close()
		if err != nil {
			log.Println("error closing output: ", err)
		}
	}
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
		log.Printf("Failed to unmarshal command: %v\n", err)
		return
	}

	if len(batch.GetCommands()) > 0 && srv.conf.PrintThroughput {
		now := time.Now().UnixNano()
		prev := atomic.SwapInt64(&srv.lastExecTime, now)
		fmt.Printf("%d, %d\n", now-prev, len(batch.GetCommands()))
	}

	for _, cmd := range batch.GetCommands() {
		_, err := srv.output.Write(cmd.Data)
		if err != nil {
			log.Printf("Error writing data: %v\n", err)
		}
		srv.mut.Lock()
		if c, ok := srv.finishedCmds[cmdID{cmd.ClientID, cmd.SequenceNumber}]; ok {
			c <- struct{}{}
		}
		srv.mut.Unlock()
	}
}
