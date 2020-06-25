package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/clientapi"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

type config struct {
	SelfID      hotstuff.ReplicaID `mapstructure:"self-id"`
	RequestRate int                `mapstructure:"request-rate"`
	PayloadSize int                `mapstructure:"payload-size"`
	MaxInflight uint64             `mapstructure:"max-inflight"`
	DataSource  string             `mapstructure:"input"`
	Benchmark   bool               `mapstructure:"benchmark"`
	ExitAfter   int                `mapstructure:"exit-after"`
	Replicas    []struct {
		ID         hotstuff.ReplicaID
		ClientAddr string `mapstructure:"client-address"`
		Pubkey     string
	}
}

func usage() {
	fmt.Printf("Usage: %s [options]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Loads configuration from ./hotstuff.toml")
	fmt.Println()
	fmt.Println("Options:")
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	help := pflag.BoolP("help", "h", false, "Prints this text.")
	cpuprofile := pflag.String("cpuprofile", "", "File to write CPU profile to")
	memprofile := pflag.String("memprofile", "", "File to write memory profile to")
	pflag.Uint32("self-id", 0, "The id for this replica.")
	pflag.Int("request-rate", 10000, "The request rate in requests/sec")
	pflag.Int("payload-size", 0, "The size of the payload in bytes")
	pflag.Uint64("max-inflight", 10000, "The maximum number of messages that the client can wait for at once")
	pflag.String("input", "", "Optional file to use for payload data")
	pflag.Bool("benchmark", false, "If enabled, the latency of each request will be printed to stdout")
	pflag.Int("exit-after", 0, "Number of seconds after which the program should exit.")
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

	var conf config
	err = viper.Unmarshal(&conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal config: %v\n", err)
		os.Exit(1)
	}

	replicaConfig := hotstuff.NewConfig()
	for _, r := range conf.Replicas {
		key, err := hotstuff.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}
		replicaConfig.Replicas[r.ID] = &hotstuff.ReplicaInfo{
			ID:      r.ID,
			Address: r.ClientAddr,
			PubKey:  key,
		}
	}

	client, err := newHotStuffClient(&conf, replicaConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start client: %v\n", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if conf.ExitAfter > 0 {
			select {
			case <-time.After(time.Duration(conf.ExitAfter) * time.Second):
			case <-signals:
				fmt.Fprintf(os.Stderr, "Exiting...\n")
			}
		} else {
			<-signals
			fmt.Fprintf(os.Stderr, "Exiting...\n")
		}
		cancel()
	}()

	err = client.SendCommands(ctx)
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Failed to send commands: %v\n", err)
		client.Close()
		os.Exit(1)
	}
	client.Close()

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

type qspec struct {
	faulty int
}

func (q *qspec) ExecCommandQF(cmd *clientapi.Command, signatures []*clientapi.Empty) (*clientapi.Empty, bool) {
	if len(signatures) < q.faulty+1 {
		return nil, false
	}
	return &clientapi.Empty{}, true
}

type hotstuffClient struct {
	inflight      uint64
	reader        io.ReadCloser
	conf          *config
	mgr           *clientapi.Manager
	replicaConfig *hotstuff.ReplicaConfig
	gorumsConfig  *clientapi.Configuration
	wg            sync.WaitGroup
}

func newHotStuffClient(conf *config, replicaConfig *hotstuff.ReplicaConfig) (*hotstuffClient, error) {
	var addrs []string
	for _, r := range replicaConfig.Replicas {
		addrs = append(addrs, r.Address)
	}
	mgr, err := clientapi.NewManager(addrs, clientapi.WithGrpcDialOptions(
		grpc.WithInsecure(),
		grpc.WithBlock(),
	),
		clientapi.WithDialTimeout(time.Minute),
	)
	if err != nil {
		return nil, err
	}
	faulty := (len(replicaConfig.Replicas) - 1) / 3
	gorumsConf, err := mgr.NewConfiguration(mgr.NodeIDs(), &qspec{faulty: faulty})
	if err != nil {
		mgr.Close()
		return nil, err
	}
	var reader io.ReadCloser
	if conf.DataSource == "" {
		reader = ioutil.NopCloser(rand.Reader)
	} else {
		f, err := os.Open(conf.DataSource)
		if err != nil {
			mgr.Close()
			return nil, err
		}
		reader = f
	}
	return &hotstuffClient{
		reader:        reader,
		conf:          conf,
		mgr:           mgr,
		replicaConfig: replicaConfig,
		gorumsConfig:  gorumsConf,
	}, nil
}

func (c *hotstuffClient) Close() {
	c.wg.Wait()
	c.mgr.Close()
	c.reader.Close()
}

func (c *hotstuffClient) SendCommands(ctx context.Context) error {
	var num uint64
	prevExecTime := time.Now().UnixNano()
	sleeptime := time.Second / time.Duration(c.conf.RequestRate)
	for {
		start := time.Now().UnixNano()
		if atomic.LoadUint64(&c.inflight) < c.conf.MaxInflight {
			atomic.AddUint64(&c.inflight, 1)
			data := make([]byte, c.conf.PayloadSize)
			n, err := c.reader.Read(data)
			if err != nil {
				return err
			}
			cmd := &clientapi.Command{
				ClientID:       uint32(c.conf.SelfID),
				SequenceNumber: num,
				Data:           data[:n],
			}
			now := time.Now().UnixNano()
			promise := c.gorumsConfig.ExecCommand(ctx, cmd)
			num++

			c.wg.Add(1)
			go func(promise *clientapi.FutureEmpty, sendTime int64) {
				_, err := promise.Get()
				atomic.AddUint64(&c.inflight, ^uint64(0))
				if err != nil {
					qcError, ok := err.(clientapi.QuorumCallError)
					if !ok || qcError.Reason != context.Canceled.Error() {
						log.Printf("Did not get enough signatures for command: %v\n", err)
					}
				}
				if c.conf.Benchmark {
					now := time.Now().UnixNano()
					prevExec := atomic.SwapInt64(&prevExecTime, now)
					fmt.Printf("%d %d\n", now-prevExec, now-sendTime)
				}
				c.wg.Done()
			}(promise, now)
		}

		timeSpent := time.Duration(time.Now().UnixNano() - start)
		select {
		case <-time.After(sleeptime - timeSpent):
		case <-ctx.Done():
			return nil
		}
	}
}
