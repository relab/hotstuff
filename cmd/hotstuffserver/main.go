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
	"github.com/golang/protobuf/ptypes"
	"github.com/relab/gorums/strictordering"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/clientapi"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
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
	grpcSrv *grpc.Server
	hs      *hotstuff.HotStuff
	pm      pacemaker.Pacemaker
	backend hotstuff.Backend

	mut          sync.Mutex
	requestIDs   map[hotstuff.Command]uint64
	finishedCmds chan hotstuff.Command

	measureMut      sync.Mutex
	lastMeasureTime time.Time
	numCommands     int
}

func newHotStuffServer(key *ecdsa.PrivateKey, conf *config, replicaConfig *hotstuff.ReplicaConfig) *hotstuffServer {
	waitDuration := time.Duration(conf.ViewTimeout/2) * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	srv := &hotstuffServer{
		ctx:          ctx,
		cancel:       cancel,
		conf:         conf,
		key:          key,
		grpcSrv:      grpc.NewServer(),
		requestIDs:   make(map[hotstuff.Command]uint64),
		finishedCmds: make(chan hotstuff.Command, conf.BatchSize*2),
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
	// Use a custom server instead of the gorums one
	strictordering.RegisterGorumsStrictOrderingServer(srv.grpcSrv, srv)
	return srv
}

func (srv *hotstuffServer) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	go srv.grpcSrv.Serve(lis)

	err = srv.backend.Start()
	if err != nil {
		return err
	}

	go srv.pm.Run(srv.ctx)

	return nil
}

func (srv *hotstuffServer) Stop() {
	srv.grpcSrv.Stop()
	srv.cancel()
	srv.hs.Close()
}

// Custom server code
func (srv *hotstuffServer) NodeStream(stream strictordering.GorumsStrictOrdering_NodeStreamServer) error {
	go func() {
		for cmd := range srv.finishedCmds {
			// compute hash
			b := []byte(cmd)
			hash := sha256.Sum256(b)
			r, s, err := ecdsa.Sign(rand.Reader, srv.key, hash[:])
			if err != nil {
				continue
			}

			// create response and marshal into Any
			sig := &clientapi.Signature{
				ReplicaID: uint32(srv.hs.GetID()),
				R:         r.Bytes(),
				S:         s.Bytes(),
			}
			any, err := ptypes.MarshalAny(sig)
			if err != nil {
				continue
			}

			// get id
			srv.mut.Lock()
			id, ok := srv.requestIDs[cmd]
			if !ok {
				srv.mut.Unlock()
				continue
			}
			delete(srv.requestIDs, cmd)
			srv.mut.Unlock()

			// send response
			resp := &strictordering.GorumsMessage{
				ID:   id,
				Data: any,
			}
			err = stream.Send(resp)
			if err != nil {
				return
			}
		}
	}()

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		// use the serialized data as cmd
		cmd := hotstuff.Command(req.GetData().GetValue())
		srv.mut.Lock()
		srv.requestIDs[cmd] = req.GetID()
		srv.mut.Unlock()
		srv.hs.AddCommand(cmd)
	}
}

func (srv *hotstuffServer) onExec(cmds []hotstuff.Command) {
	if srv.conf.PrintThroughput {
		now := time.Now()
		srv.measureMut.Lock()
		srv.numCommands += len(cmds)
		if diff := now.Sub(srv.lastMeasureTime); diff > time.Duration(srv.conf.Interval)*time.Millisecond {
			fmt.Printf("%d, %d\n", diff, srv.numCommands)
			srv.numCommands = 0
			srv.lastMeasureTime = now
		}
		srv.measureMut.Unlock()
	}

	for _, cmd := range cmds {
		if srv.conf.PrintCommands {
			m := new(clientapi.Command)
			protobuf.Unmarshal([]byte(cmd), m)
			fmt.Printf("%s", m.Data)
		}
		srv.finishedCmds <- cmd
	}
}
