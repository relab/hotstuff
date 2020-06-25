package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	protobuf "github.com/golang/protobuf/proto"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/clientapi"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/proto"
)

type config struct {
	Privkey         string
	SelfID          hotstuff.ReplicaID   `mapstructure:"self-id"`
	PmType          string               `mapstructure:"pacemaker"`
	LeaderID        hotstuff.ReplicaID   `mapstructure:"leader-id"`
	Schedule        []hotstuff.ReplicaID `mapstructure:"leader-schedule"`
	ViewChange      int                  `mapstructure:"view-change"`
	ViewTimeout     int                  `mapstructure:"view-timeout"`
	BatchSize       int                  `mapstructure:"batch-size"`
	PrintThroughput bool                 `mapstructure:"print-throughput"`
	PrintCommands   bool                 `mapstructure:"print-commands"`
	Interval        int
	Output          string
	Replicas        []struct {
		ID         hotstuff.ReplicaID
		PeerAddr   string `mapstructure:"peer-address"`
		ClientAddr string `mapstructure:"client-address"`
		Pubkey     string
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
	pflag.Uint32("self-id", 0, "The id for this replica.")
	pflag.Int("view-change", 100, "How many views before leader change with round-robin pacemaker")
	pflag.Int("batch-size", 100, "How many commands are batched together for each proposal")
	pflag.Int("view-timeout", 1000, "How many milliseconds before a view is timed out")
	pflag.String("privkey", "", "The path to the private key file")
	pflag.Bool("print-commands", false, "Commands will be printed to stdout")
	pflag.Bool("print-throughput", false, "Throughput measurements will be printed stdout")
	pflag.Int("interval", 1000, "Throughput measurement interval in milliseconds")
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

	var conf config
	err = viper.Unmarshal(&conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal config: %v\n", err)
		os.Exit(1)
	}

	privkey, err := hotstuff.ReadPrivateKeyFile(conf.Privkey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key file: %v\n", err)
		os.Exit(1)
	}

	var clientAddress string

	replicaConfig := hotstuff.NewConfig()
	replicaConfig.BatchSize = conf.BatchSize
	for _, r := range conf.Replicas {
		key, err := hotstuff.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}
		replicaConfig.Replicas[r.ID] = &hotstuff.ReplicaInfo{
			ID:      r.ID,
			Address: r.PeerAddr,
			PubKey:  key,
		}
		if r.ID == conf.SelfID {
			clientAddress = r.ClientAddr
		}
	}
	replicaConfig.QuorumSize = len(replicaConfig.Replicas) - (len(replicaConfig.Replicas)-1)/3

	srv := newHotStuffServer(privkey, &conf, replicaConfig)
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
	key       *ecdsa.PrivateKey
	conf      *config
	gorumsSrv *clientapi.GorumsServer
	hs        *hotstuff.HotStuff
	backend   hotstuff.Backend
	pm        interface {
		Run(context.Context)
	}

	mut          sync.Mutex
	finishedCmds map[cmdID]chan struct{}

	lastExecTime int64
}

func newHotStuffServer(key *ecdsa.PrivateKey, conf *config, replicaConfig *hotstuff.ReplicaConfig) *hotstuffServer {
	waitDuration := time.Duration(conf.ViewTimeout/2) * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	srv := &hotstuffServer{
		ctx:          ctx,
		cancel:       cancel,
		conf:         conf,
		key:          key,
		gorumsSrv:    clientapi.NewGorumsServer(),
		finishedCmds: make(map[cmdID]chan struct{}),
		lastExecTime: time.Now().UnixNano(),
	}
	srv.backend = gorumshotstuff.New(time.Minute, time.Duration(conf.ViewTimeout)*time.Millisecond)
	srv.hs = hotstuff.New(conf.SelfID, key, replicaConfig, srv.backend, waitDuration, srv.onExec)
	switch conf.PmType {
	case "fixed":
		srv.pm = pacemaker.NewFixedLeader(srv.hs, conf.LeaderID)
	case "round-robin":
		srv.pm = pacemaker.NewRoundRobin(
			srv.hs, conf.ViewChange, conf.Schedule, time.Duration(conf.ViewTimeout)*time.Millisecond,
		)
	default:
		fmt.Fprintf(os.Stderr, "Invalid pacemaker type: '%s'\n", conf.PmType)
		os.Exit(1)
	}
	// Use a custom server instead of the gorums one
	srv.gorumsSrv.RegisterHotStuffSMRServer(srv)
	return srv
}

func (srv *hotstuffServer) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	err = srv.backend.Start()
	if err != nil {
		return err
	}

	go srv.gorumsSrv.Serve(lis)
	go srv.pm.Run(srv.ctx)

	return nil
}

func (srv *hotstuffServer) Stop() {
	srv.gorumsSrv.Stop()
	srv.cancel()
	srv.hs.Close()
}

func (srv *hotstuffServer) ExecCommand(cmd *clientapi.Command, out chan<- *clientapi.Empty) {
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
	}
	srv.hs.AddCommand(hotstuff.Command(b))

	go func(id cmdID, finished chan struct{}) {
		<-finished

		srv.mut.Lock()
		delete(srv.finishedCmds, id)
		srv.mut.Unlock()

		// send response
		out <- &clientapi.Empty{}
	}(id, finished)
}

func (srv *hotstuffServer) onExec(cmds []hotstuff.Command) {
	if len(cmds) > 0 && srv.conf.PrintThroughput {
		now := time.Now().UnixNano()
		prev := atomic.SwapInt64(&srv.lastExecTime, now)
		fmt.Printf("%d, %d\n", now-prev, len(cmds))
	}

	for _, cmd := range cmds {
		m := new(clientapi.Command)
		err := protobuf.Unmarshal([]byte(cmd), m)
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
		fmt.Printf("%s", m.Data)
	}
}
