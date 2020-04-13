package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	protobuf "github.com/golang/protobuf/proto"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/clientapi"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	configFile := pflag.String("config", "", "The path to the config file")
	help := pflag.BoolP("help", "h", false, "Prints this text.")
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
}

type hotstuffServer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	key     *ecdsa.PrivateKey
	conf    *config
	gorums  *clientapi.GorumsServer
	hs      *hotstuff.HotStuff
	pm      pacemaker.Pacemaker
	backend hotstuff.Backend

	mut             sync.Mutex
	waitingRequests map[hotstuff.Command]chan struct{}

	measureMut      sync.Mutex
	lastMeasureTime time.Time
	numCommands     int
}

func newHotStuffServer(key *ecdsa.PrivateKey, conf *config, replicaConfig *hotstuff.ReplicaConfig) *hotstuffServer {
	waitDuration := time.Duration(conf.ViewTimeout/2) * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	srv := &hotstuffServer{
		ctx:             ctx,
		cancel:          cancel,
		conf:            conf,
		key:             key,
		gorums:          clientapi.NewGorumsServer(),
		waitingRequests: make(map[hotstuff.Command]chan struct{}),
	}
	srv.backend = gorumshotstuff.New(time.Minute, time.Duration(conf.ViewTimeout)*time.Millisecond)
	srv.hs = hotstuff.New(conf.SelfID, key, replicaConfig, srv.backend, waitDuration, srv.onExec)
	switch conf.PmType {
	case "fixed":
		srv.pm = &pacemaker.FixedLeaderPacemaker{HotStuff: srv.hs, Leader: conf.LeaderID}
	case "round-robin":
		srv.pm = &pacemaker.RoundRobinPacemaker{
			HotStuff:       srv.hs,
			Schedule:       conf.Schedule,
			TermLength:     conf.ViewChange,
			NewViewTimeout: time.Duration(conf.ViewTimeout) * time.Millisecond,
		}
	default:
		fmt.Fprintf(os.Stderr, "Invalid pacemaker type: '%s'\n", conf.PmType)
		os.Exit(1)
	}
	srv.gorums.RegisterExecCommandHandler(srv)
	return srv
}

func (srv *hotstuffServer) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	go srv.gorums.Serve(lis)

	err = srv.backend.Start()
	if err != nil {
		return err
	}

	go srv.pm.Run(srv.ctx)

	return nil
}

func (srv *hotstuffServer) Stop() {
	srv.gorums.Stop()
	srv.cancel()
	srv.hs.Close()
}

func (srv *hotstuffServer) ExecCommand(ctx context.Context, cmd *clientapi.Command) *clientapi.Signature {
	// marshal cmd into a string and send it to hotstuff
	_c, err := protobuf.Marshal(cmd)
	if err != nil {
		return nil
	}
	c := hotstuff.Command(_c)

	signal := make(chan struct{}, 1)
	srv.mut.Lock()
	srv.waitingRequests[c] = signal
	srv.mut.Unlock()

	defer func() {
		srv.mut.Lock()
		delete(srv.waitingRequests, c)
		srv.mut.Unlock()
	}()

	srv.hs.AddCommand(c)

	select {
	case <-signal:
		hash := sha256.Sum256(_c)
		r, s, err := ecdsa.Sign(rand.Reader, srv.key, hash[:])
		if err != nil {
			return nil
		}
		return &clientapi.Signature{
			ReplicaID: uint32(srv.conf.SelfID),
			R:         r.Bytes(),
			S:         s.Bytes(),
		}
	case <-ctx.Done():
		return nil
	// probably won't need to check this one aswell
	case <-srv.ctx.Done():
		return nil
	}
}

func (srv *hotstuffServer) onExec(cmds []hotstuff.Command) {
	if srv.conf.PrintThroughput {
		now := time.Now()
		srv.measureMut.Lock()
		srv.numCommands += len(cmds)
		if now.Sub(srv.lastMeasureTime) > time.Duration(srv.conf.Interval)*time.Millisecond {
			fmt.Printf("%s, %d\n", now.Format("15:04:05.000"), srv.numCommands)
			srv.numCommands = 0
			srv.lastMeasureTime = now
		}
		srv.measureMut.Unlock()
	}

	srv.mut.Lock()
	for _, cmd := range cmds {
		if srv.conf.PrintCommands {
			m := new(clientapi.Command)
			protobuf.Unmarshal([]byte(cmd), m)
			fmt.Printf("%s", m.Data)
		}

		if c, ok := srv.waitingRequests[cmd]; ok {
			c <- struct{}{}
		}
	}
	srv.mut.Unlock()
}
