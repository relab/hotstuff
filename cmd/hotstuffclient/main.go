package main

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/gorums/benchmark"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/internal/cli"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type options struct {
	SelfID      hotstuff.ID `mapstructure:"self-id"`
	RateLimit   int         `mapstructure:"rate-limit"`
	PayloadSize int         `mapstructure:"payload-size"`
	MaxInflight uint64      `mapstructure:"max-inflight"`
	DataSource  string      `mapstructure:"input"`
	Benchmark   bool        `mapstructure:"benchmark"`
	ExitAfter   int         `mapstructure:"exit-after"`
	TLS         bool
	RootCAs     []string `mapstructure:"root-cas"`
	Replicas    []struct {
		ID         hotstuff.ID
		ClientAddr string `mapstructure:"client-address"`
		Pubkey     string
		Cert       string
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
	pflag.Int("rate-limit", 0, "Limit the request-rate to approximately (in requests per second).")
	pflag.Int("payload-size", 0, "The size of the payload in bytes")
	pflag.Uint64("max-inflight", 10000, "The maximum number of messages that the client can wait for at once")
	pflag.String("input", "", "Optional file to use for payload data")
	pflag.Bool("benchmark", false, "If enabled, a BenchmarkData protobuf will be written to stdout.")
	pflag.Int("exit-after", 0, "Number of seconds after which the program should exit.")
	pflag.Bool("tls", false, "Enable TLS")
	pflag.Parse()

	if *help {
		pflag.Usage()
		os.Exit(0)
	}

	profileStop, err := profiling.StartProfilers(*cpuprofile, *memprofile, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start profilers: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		err := profileStop()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop profilers: %v\n", err)
			os.Exit(1)
		}
	}()

	var conf options
	err = cli.ReadConfig(&conf, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	start(ctx, &conf)
}

func start(ctx context.Context, conf *options) {
	var creds credentials.TransportCredentials
	if conf.TLS {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get system cert pool: %v\n", err)
			os.Exit(1)
		}
		for _, ca := range conf.RootCAs {
			cert, err := os.ReadFile(ca)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read CA: %v\n", err)
				os.Exit(1)
			}
			if !rootCAs.AppendCertsFromPEM(cert) {
				fmt.Fprintf(os.Stderr, "Failed to decode CA\n")
				os.Exit(1)
			}
		}
		creds = credentials.NewClientTLSFromCert(rootCAs, "")
	}

	replicaConfig := config.NewConfig(0, nil, creds)
	for _, r := range conf.Replicas {
		key, err := crypto.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}
		replicaConfig.Replicas[r.ID] = &config.ReplicaInfo{
			ID:      r.ID,
			Address: r.ClientAddr,
			PubKey:  key,
		}
	}

	client, err := newHotStuffClient(conf, replicaConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start client: %v\n", err)
		os.Exit(1)
	}

	err = client.SendCommands(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "Failed to send commands: %v\n", err)
		client.Close()
		os.Exit(1)
	}
	client.Close()

	stats := client.GetStats()
	throughput := stats.Throughput
	latency := stats.LatencyAvg / float64(time.Millisecond)
	latencySD := math.Sqrt(stats.LatencyVar) / float64(time.Millisecond)

	if !conf.Benchmark {
		fmt.Printf("Throughput (ops/sec): %.2f, Latency (ms): %.2f, Latency Std.dev (ms): %.2f\n",
			throughput,
			latency,
			latencySD,
		)
	} else {
		client.data.MeasuredThroughput = throughput
		client.data.MeasuredLatency = latency
		client.data.LatencyVariance = math.Pow(latencySD, 2) // variance in ms^2
		b, err := proto.Marshal(client.data)
		if err != nil {
			log.Fatalf("Could not marshal benchmarkdata: %v\n", err)
		}
		_, err = os.Stdout.Write(b)
		if err != nil {
			log.Fatalf("Could not write data: %v\n", err)
		}
	}
}

type qspec struct {
	faulty int
}

func (q *qspec) ExecCommandQF(_ *client.Command, signatures map[uint32]*empty.Empty) (*empty.Empty, bool) {
	if len(signatures) < q.faulty+1 {
		return nil, false
	}
	return &empty.Empty{}, true
}

type hotstuffClient struct {
	inflight      uint64
	reader        io.ReadCloser
	conf          *options
	mgr           *client.Manager
	replicaConfig *config.ReplicaConfig
	gorumsConfig  *client.Configuration
	wg            sync.WaitGroup
	stats         benchmark.Stats       // records latency and throughput
	data          *client.BenchmarkData // stores time and duration for each command
}

func newHotStuffClient(conf *options, replicaConfig *config.ReplicaConfig) (*hotstuffClient, error) {
	nodes := make(map[string]uint32, len(replicaConfig.Replicas))
	for _, r := range replicaConfig.Replicas {
		nodes[r.Address] = uint32(r.ID)
	}

	grpcOpts := []grpc.DialOption{grpc.WithBlock()}

	if conf.TLS {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(replicaConfig.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgr := client.NewManager(gorums.WithGrpcDialOptions(grpcOpts...),
		gorums.WithDialTimeout(time.Minute),
	)
	faulty := (len(replicaConfig.Replicas) - 1) / 3
	gorumsConf, err := mgr.NewConfiguration(&qspec{faulty: faulty}, gorums.WithNodeMap(nodes))
	if err != nil {
		mgr.Close()
		return nil, err
	}
	var reader io.ReadCloser
	if conf.DataSource == "" {
		reader = io.NopCloser(rand.Reader)
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
		data:          &client.BenchmarkData{},
	}, nil
}

func (c *hotstuffClient) Close() {
	c.mgr.Close()
	err := c.reader.Close()
	if err != nil {
		log.Println(err)
	}
}

func (c *hotstuffClient) GetStats() *benchmark.Result {
	return c.stats.GetResult()
}

func (c *hotstuffClient) SendCommands(ctx context.Context) error {
	num := uint64(1)
	var sleeptime time.Duration
	if c.conf.RateLimit > 0 {
		sleeptime = time.Second / time.Duration(c.conf.RateLimit)
	}

	defer c.stats.End()
	defer c.wg.Wait()
	c.stats.Start()

	for {
		if atomic.LoadUint64(&c.inflight) < c.conf.MaxInflight {
			atomic.AddUint64(&c.inflight, 1)
			data := make([]byte, c.conf.PayloadSize)
			n, err := c.reader.Read(data)
			if err != nil {
				return err
			}
			cmd := &client.Command{
				ClientID:       uint32(c.conf.SelfID),
				SequenceNumber: num,
				Data:           data[:n],
			}
			now := time.Now()
			promise := c.gorumsConfig.ExecCommand(ctx, cmd)
			num++

			c.wg.Add(1)
			go func(promise *client.AsyncEmpty, sendTime time.Time) {
				_, err := promise.Get()
				atomic.AddUint64(&c.inflight, ^uint64(0))
				if err != nil {
					qcError, ok := err.(gorums.QuorumCallError)
					if !ok || qcError.Reason != context.Canceled.Error() {
						log.Printf("Did not get enough replies for command: %v\n", err)
					}
				}
				duration := time.Since(sendTime)
				c.stats.AddLatency(duration)
				if c.conf.Benchmark {
					c.data.Stats = append(c.data.Stats, &client.CommandStats{
						StartTime: timestamppb.New(sendTime),
						Duration:  durationpb.New(duration),
					})
				}
				c.wg.Done()
			}(promise, now)
		}

		if c.conf.RateLimit > 0 {
			time.Sleep(sleeptime)
		}

		err := ctx.Err()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
