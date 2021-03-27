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
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/gorums/benchmark"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func runClient(ctx context.Context, conf *options) {
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
	reader        io.Reader
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

	gorumsConf, err := mgr.NewConfiguration(&qspec{faulty: hotstuff.NumFaulty(len(conf.Replicas))}, gorums.WithNodeMap(nodes))
	if err != nil {
		mgr.Close()
		return nil, err
	}

	client := &hotstuffClient{
		conf:          conf,
		mgr:           mgr,
		replicaConfig: replicaConfig,
		gorumsConfig:  gorumsConf,
		data:          &client.BenchmarkData{},
	}

	if conf.Input != "" {
		client.reader, err = os.Open(conf.Input)
		if err != nil {
			return nil, err
		}
	} else {
		client.reader = rand.Reader
	}

	return client, nil
}

func (c *hotstuffClient) Close() {
	c.mgr.Close()
	if closer, ok := c.reader.(io.Closer); ok {
		closer.Close()
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
