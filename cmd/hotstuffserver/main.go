package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/felixge/fgprof"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/data"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type options struct {
	Privkey         string
	Cert            string
	SelfID          config.ReplicaID   `mapstructure:"self-id"`
	PmType          string             `mapstructure:"pacemaker"`
	LeaderID        config.ReplicaID   `mapstructure:"leader-id"`
	Schedule        []config.ReplicaID `mapstructure:"leader-schedule"`
	ViewChange      int                `mapstructure:"view-change"`
	ViewTimeout     int                `mapstructure:"view-timeout"`
	BatchSize       int                `mapstructure:"batch-size"`
	PrintThroughput bool               `mapstructure:"print-throughput"`
	PrintCommands   bool               `mapstructure:"print-commands"`
	ClientAddr      string             `mapstructure:"client-listen"`
	PeerAddr        string             `mapstructure:"peer-listen"`
	TLS             bool
	Interval        int
	Output          string
	Replicas        []struct {
		ID         config.ReplicaID
		PeerAddr   string `mapstructure:"peer-address"`
		ClientAddr string `mapstructure:"client-address"`
		Pubkey     string
		Cert       string
	}
}

func usage() {
	fmt.Printf("Usage: %s [options]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Loads configuration from ./hotstuff.toml and file specified by --config")
	fmt.Println()
	fmt.Println("Options:")
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// some configuration options can be set using flags
	help := pflag.BoolP("help", "h", false, "Prints this text.")
	configFile := pflag.String("config", "", "The path to the config file")
	cpuprofile := pflag.String("cpuprofile", "", "File to write CPU profile to")
	memprofile := pflag.String("memprofile", "", "File to write memory profile to")
	fullprofile := pflag.String("fullprofile", "", "File to write fgprof profile to")
	traceFile := pflag.String("trace", "", "File to write execution trace to")
	pflag.Uint32("self-id", 0, "The id for this replica.")
	pflag.Int("view-change", 100, "How many views before leader change with round-robin pacemaker")
	pflag.Int("batch-size", 100, "How many commands are batched together for each proposal")
	pflag.Int("view-timeout", 1000, "How many milliseconds before a view is timed out")
	pflag.String("privkey", "", "The path to the private key file")
	pflag.String("cert", "", "Path to the certificate")
	pflag.Bool("print-commands", false, "Commands will be printed to stdout")
	pflag.Bool("print-throughput", false, "Throughput measurements will be printed stdout")
	pflag.Int("interval", 1000, "Throughput measurement interval in milliseconds")
	pflag.Bool("tls", false, "Enable TLS")
	pflag.String("client-listen", "", "Override the listen address for the client server")
	pflag.String("peer-listen", "", "Override the listen address for the replica (peer) server")
	pflag.Parse()

	if *help {
		pflag.Usage()
		os.Exit(0)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if *fullprofile != "" {
		f, err := os.Create(*fullprofile)
		if err != nil {
			log.Fatal("Could not create fgprof profile: ", err)
		}
		defer f.Close()
		stop := fgprof.Start(f, fgprof.FormatPprof)

		defer func() {
			err := stop()
			if err != nil {
				log.Fatal("Could not write fgprof profile: ", err)
			}
		}()
	}

	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			log.Fatal("Could not create trace file: ", err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			log.Fatal("Failed to start trace: ", err)
		}
		defer trace.Stop()
	}

	viper.BindPFlags(pflag.CommandLine)

	// read main config file in working dir
	viper.SetConfigName("hotstuff")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read config: %v\n", err)
		os.Exit(1)
	}

	// read secondary config file
	if *configFile != "" {
		viper.SetConfigFile(*configFile)
		err = viper.MergeInConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read secondary config file: %v\n", err)
			os.Exit(1)
		}
	}

	var conf options
	err = viper.Unmarshal(&conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal config: %v\n", err)
		os.Exit(1)
	}

	privkey, err := data.ReadPrivateKeyFile(conf.Privkey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key file: %v\n", err)
		os.Exit(1)
	}

	var cert *tls.Certificate
	if conf.TLS {
		if conf.Cert == "" {
			for _, replica := range conf.Replicas {
				if replica.ID == conf.SelfID {
					conf.Cert = replica.Cert
				}
			}
		}
		certB, err := ioutil.ReadFile(conf.Cert)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read certificate: %v\n", err)
			os.Exit(1)
		}
		// read in the private key again, but in PEM format
		pkPEM, err := ioutil.ReadFile(conf.Privkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read private key: %v\n", err)
			os.Exit(1)
		}

		tlsCert, err := tls.X509KeyPair(certB, pkPEM)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse certificate: %v\n", err)
			os.Exit(1)
		}
		cert = &tlsCert
	}

	var clientAddress string

	replicaConfig := config.NewConfig(conf.SelfID, privkey, cert)
	replicaConfig.BatchSize = conf.BatchSize
	for _, r := range conf.Replicas {
		key, err := data.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}

		var cert *x509.Certificate
		if conf.TLS {
			cert, err = data.ReadCertFile(r.Cert)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read certificate: %v\n", err)
				os.Exit(1)
			}
		}

		info := &config.ReplicaInfo{
			ID:      r.ID,
			Address: r.PeerAddr,
			PubKey:  key,
			Cert:    cert,
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
	replicaConfig.QuorumSize = len(replicaConfig.Replicas) - (len(replicaConfig.Replicas)-1)/3

	srv := newHotStuffServer(&conf, replicaConfig)
	err = srv.Start(clientAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start HotStuff: %v\n", err)
		os.Exit(1)
	}

	<-signals
	fmt.Fprintf(os.Stderr, "Exiting...\n")
	srv.Stop()

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}

// cmdID is a unique identifier for a command
type cmdID struct {
	clientID    uint32
	sequenceNum uint64
}

type hotstuffServer struct {
	ctx       context.Context
	cancel    context.CancelFunc
	conf      *options
	gorumsSrv *gorums.Server
	hs        *hotstuff.HotStuff
	pm        interface {
		Run(context.Context)
	}

	mut          sync.Mutex
	finishedCmds map[cmdID]chan struct{}

	lastExecTime int64
}

func newHotStuffServer(conf *options, replicaConfig *config.ReplicaConfig) *hotstuffServer {
	ctx, cancel := context.WithCancel(context.Background())

	serverOpts := []gorums.ServerOption{}
	grpcServerOpts := []grpc.ServerOption{}

	if conf.TLS {
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(credentials.NewServerTLSFromCert(replicaConfig.Cert)))
	}

	serverOpts = append(serverOpts, gorums.WithGRPCServerOptions(grpcServerOpts...))

	srv := &hotstuffServer{
		ctx:          ctx,
		cancel:       cancel,
		conf:         conf,
		gorumsSrv:    gorums.NewServer(serverOpts...),
		finishedCmds: make(map[cmdID]chan struct{}),
		lastExecTime: time.Now().UnixNano(),
	}
	var pm hotstuff.Pacemaker
	switch conf.PmType {
	case "fixed":
		pm = pacemaker.NewFixedLeader(conf.LeaderID)
	case "round-robin":
		pm = pacemaker.NewRoundRobin(
			conf.ViewChange, conf.Schedule, time.Duration(conf.ViewTimeout)*time.Millisecond,
		)
	default:
		fmt.Fprintf(os.Stderr, "Invalid pacemaker type: '%s'\n", conf.PmType)
		os.Exit(1)
	}
	srv.hs = hotstuff.New(replicaConfig, pm, conf.TLS, time.Minute, time.Duration(conf.ViewTimeout)*time.Millisecond)
	srv.pm = pm.(interface{ Run(context.Context) })
	// Use a custom server instead of the gorums one
	client.RegisterClientServer(srv.gorumsSrv, srv)
	return srv
}

func (srv *hotstuffServer) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	err = srv.hs.Start()
	if err != nil {
		return err
	}

	go srv.gorumsSrv.Serve(lis)
	go srv.pm.Run(srv.ctx)
	go srv.onExec()

	return nil
}

func (srv *hotstuffServer) Stop() {
	srv.gorumsSrv.Stop()
	srv.cancel()
	srv.hs.Close()
}

func (srv *hotstuffServer) ExecCommand(_ context.Context, cmd *client.Command, out func(*client.Empty, error)) {
	finished := make(chan struct{})
	id := cmdID{cmd.ClientID, cmd.SequenceNumber}
	srv.mut.Lock()
	srv.finishedCmds[id] = finished
	srv.mut.Unlock()
	// marshal the message back to a byte so that HotStuff can process it.
	// TODO: think of a better way to do this.
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(cmd)
	if err != nil {
		log.Fatalf("Failed to marshal command: %v", err)
		out(nil, status.Errorf(codes.InvalidArgument, "Failed to marshal command: %v", err))
	}
	srv.hs.AddCommand(data.Command(b))

	go func(id cmdID, finished chan struct{}) {
		<-finished

		srv.mut.Lock()
		delete(srv.finishedCmds, id)
		srv.mut.Unlock()

		// send response
		out(&client.Empty{}, nil)
	}(id, finished)
}

func (srv *hotstuffServer) onExec() {
	for cmds := range srv.hs.GetExec() {
		if len(cmds) > 0 && srv.conf.PrintThroughput {
			now := time.Now().UnixNano()
			prev := atomic.SwapInt64(&srv.lastExecTime, now)
			fmt.Printf("%d, %d\n", now-prev, len(cmds))
		}

		for _, cmd := range cmds {
			m := new(client.Command)
			err := proto.Unmarshal([]byte(cmd), m)
			if err != nil {
				log.Printf("Failed to unmarshal command: %v\n", err)
			}
			if srv.conf.PrintCommands {
				fmt.Printf("%s", m.Data)
			}
			srv.mut.Lock()
			if c, ok := srv.finishedCmds[cmdID{m.ClientID, m.SequenceNumber}]; ok {
				c <- struct{}{}
			}
			srv.mut.Unlock()
		}
	}
}
