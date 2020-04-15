package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	protobuf "github.com/golang/protobuf/proto"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/clientapi"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

type config struct {
	RequestRate    int    `mapstructure:"request-rate"`
	PayloadSize    int    `mapstructure:"payload-size"`
	MaxInflight    uint64 `mapstructure:"max-inflight"`
	DataSource     string `mapstructure:"input"`
	PrintLatencies bool   `mapstructure:"print-latencies"`
	ExitAfter      int    `mapstructure:"exit-after"`
	Replicas       []struct {
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
	pflag.Int("request-rate", 10, "The request rate in Kops/sec")
	pflag.Int("payload-size", 0, "The size of the payload in bytes")
	pflag.Uint64("max-inflight", 10000, "The maximum number of messages that the client can wait for at once")
	pflag.String("input", "", "Optional file to use for payload data")
	pflag.Bool("print-latencies", false, "If enabled, the latency of each request will be printed to stdout")
	pflag.Int("exit-after", 0, "Number of seconds after which the program should exit.")
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
		err := client.SendCommands(ctx)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Failed to send commands: %v\n", err)
			client.Close()
			os.Exit(1)
		}
		client.Close()
		os.Exit(0)
	}()

	if conf.ExitAfter > 0 {
		select {
		case <-time.After(time.Duration(conf.ExitAfter) * time.Second):
		case <-signals:
		}
	} else {
		<-signals
	}

	fmt.Fprintf(os.Stderr, "Exiting...\n")

	cancel()
}

type qspec struct {
	conf   *hotstuff.ReplicaConfig
	faulty int
}

func (q *qspec) ExecCommandQF(cmd *clientapi.Command, signatures []*clientapi.Signature) (*clientapi.Empty, bool) {
	if len(signatures) < q.faulty+1 {
		return nil, false
	}
	b, err := protobuf.Marshal(cmd)
	if err != nil {
		return nil, false
	}
	hash := sha256.Sum256(b)
	verified := 0
	for _, signature := range signatures {
		r := new(big.Int)
		r.SetBytes(signature.R)
		s := new(big.Int)
		s.SetBytes(signature.S)
		pkey := q.conf.Replicas[hotstuff.ReplicaID(signature.ReplicaID)].PubKey
		if ecdsa.Verify(pkey, hash[:], r, s) {
			verified++
		}
	}
	if verified < q.faulty+1 {
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
	gorumsConf, err := mgr.NewConfiguration(mgr.NodeIDs(), &qspec{conf: replicaConfig, faulty: faulty})
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
	sleeptime := time.Second / time.Duration(c.conf.RequestRate*1000)
	for {
		if atomic.LoadUint64(&c.inflight) < c.conf.MaxInflight {
			atomic.AddUint64(&c.inflight, 1)
			data := make([]byte, c.conf.PayloadSize)
			n, err := c.reader.Read(data)
			if err != nil {
				return err
			}
			cmd := &clientapi.Command{
				Timestamp: time.Now().UnixNano(),
				Data:      data[:n],
			}
			promise, err := c.gorumsConfig.ExecCommand(ctx, cmd)
			if err != nil {
				continue
			}
			atomic.AddUint64(&c.inflight, ^uint64(0))

			go func(cmd *clientapi.Command, promise *clientapi.FutureEmpty) {
				c.wg.Add(1)
				_, err := promise.Get()
				if err != nil {
					log.Printf("Did not get enough signatures for command: %v\n", err)
				}
				now := time.Now().UnixNano()
				if c.conf.PrintLatencies {
					fmt.Println(now - cmd.GetTimestamp())
				}
				c.wg.Done()
			}(cmd, promise)
		}

		select {
		case <-time.After(sleeptime):
		case <-ctx.Done():
			return nil
		}
	}
}
