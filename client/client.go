package main

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/gorums/benchmark"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type qspec struct {
	faulty int
}

func (q *qspec) ExecCommandQF(_ *clientpb.Command, signatures map[uint32]*empty.Empty) (*empty.Empty, bool) {
	if len(signatures) < q.faulty+1 {
		return nil, false
	}
	return &empty.Empty{}, true
}

type pendingCmd struct {
	sequenceNumber uint64
	sendTime       time.Time
	promise        *clientpb.AsyncEmpty
}

// Client is a hotstuff client.
type Client struct {
	logger        logging.Logger
	conf          *orchestrationpb.ClientRunConfig
	replicaConfig *config.ReplicaConfig
	mgr           *clientpb.Manager
	gorumsConfig  *clientpb.Configuration

	mut              sync.Mutex
	highestCommitted uint64 // highest sequence number acknowledged by the replicas
	pendingCmds      chan pendingCmd
	done             chan struct{}
	reader           io.Reader
	stats            benchmark.Stats         // records latency and throughput
	data             *clientpb.BenchmarkData // stores time and duration for each command
}

// New returns a new Client.
func New(conf *orchestrationpb.ClientRunConfig) (*Client, error) {
	logger := logging.New(fmt.Sprintf("cli%d", conf.GetID()))

	var creds credentials.TransportCredentials
	if conf.GetUseTLS() {
		rootCA := x509.NewCertPool()
		rootCA.AppendCertsFromPEM(conf.GetCertificateAuthority())
		creds = credentials.NewClientTLSFromCert(rootCA, "")
	}

	replicaConfig := config.NewConfig(consensus.ID(conf.GetID()), nil, creds)
	for id, r := range conf.GetReplicas() {
		key, err := keygen.ParsePublicKey(r.GetPublicKey())
		if err != nil {
			logger.Panicf("Failed to parse public key:", err)
			return nil, err
		}
		replicaConfig.Replicas[consensus.ID(id)] = &config.ReplicaInfo{
			ID:      consensus.ID(id),
			Address: r.GetAddress(),
			PubKey:  key,
		}
	}

	nodes := make(map[string]uint32, len(replicaConfig.Replicas))
	for _, r := range replicaConfig.Replicas {
		nodes[r.Address] = uint32(r.ID)
	}

	grpcOpts := []grpc.DialOption{grpc.WithBlock()}

	if conf.GetUseTLS() {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(replicaConfig.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	mgr := clientpb.NewManager(gorums.WithGrpcDialOptions(grpcOpts...),
		gorums.WithDialTimeout(time.Minute),
	)

	gorumsConf, err := mgr.NewConfiguration(&qspec{faulty: consensus.NumFaulty(len(conf.Replicas))}, gorums.WithNodeMap(nodes))
	if err != nil {
		mgr.Close()
		return nil, err
	}

	client := &Client{
		logger:           logger,
		conf:             conf,
		replicaConfig:    replicaConfig,
		mgr:              mgr,
		gorumsConfig:     gorumsConf,
		pendingCmds:      make(chan pendingCmd, conf.GetMaxConcurrent()),
		highestCommitted: 1,
		done:             make(chan struct{}),
		data:             &clientpb.BenchmarkData{},
	}

	// TODO
	// if conf.Input != "" {
	// 	client.reader, err = os.Open(conf.Input)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// } else {
	client.reader = rand.Reader
	// }

	return client, nil
}

// Run runs the client until the context is closed.
func (c *Client) Run(ctx context.Context) {
	c.logger.Info("Starting to send commands")
	go c.handleCommands(ctx)
	err := c.sendCommands(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		c.logger.Panicf("Failed to send commands: %v", err)
	}
	c.close()

	c.logger.Info("Done sending commands")
}

func (c *Client) close() {
	c.mgr.Close()
	if closer, ok := c.reader.(io.Closer); ok {
		err := closer.Close()
		if err != nil {
			c.logger.Warn("Failed to close reader: ", err)
		}
	}
}

// GetStats returns the results of the benchmark.
func (c *Client) GetStats() *benchmark.Result {
	return c.stats.GetResult()
}

func (c *Client) sendCommands(ctx context.Context) error {
	num := uint64(1)
	var lastCommand uint64 = math.MaxUint64

	defer c.stats.End()
	c.stats.Start()

	for {
		if ctx.Err() != nil {
			break
		}

		// annoyingly, we need a mutex here to prevent the data race detector from complaining.
		c.mut.Lock()
		shouldStop := lastCommand <= c.highestCommitted
		c.mut.Unlock()

		if shouldStop {
			break
		}

		data := make([]byte, c.conf.PayloadSize)
		n, err := c.reader.Read(data)
		if err != nil && err != io.EOF {
			// if we get an error other than EOF
			return err
		} else if err == io.EOF && n == 0 && lastCommand > num {
			lastCommand = num
			c.logger.Info("Reached end of file. Sending empty commands until last command is executed...")
		}

		cmd := &clientpb.Command{
			ClientID:       uint32(c.conf.GetID()),
			SequenceNumber: num,
			Data:           data[:n],
		}

		promise := c.gorumsConfig.ExecCommand(ctx, cmd)

		num++
		c.pendingCmds <- pendingCmd{sequenceNumber: num, sendTime: time.Now(), promise: promise}

		if num%100 == 0 {
			c.logger.Infof("%d commands sent", num)
		}

	}
	return nil
}

// handleCommands will get pending commands from the pendingCmds channel and then
// handle them as they become acknowledged by the replicas. We expect the commands to be
// acknowledged in the order that they were sent.
func (c *Client) handleCommands(ctx context.Context) {
	for {
		var (
			cmd pendingCmd
			ok  bool
		)
		select {
		case cmd, ok = <-c.pendingCmds:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
		_, err := cmd.promise.Get()
		if err != nil {
			qcError, ok := err.(gorums.QuorumCallError)
			if !ok || qcError.Reason != context.Canceled.Error() {
				c.logger.Warnf("Did not get enough replies for command: %v\n", err)
			}
		}
		c.mut.Lock()
		if cmd.sequenceNumber > c.highestCommitted {
			c.highestCommitted = cmd.sequenceNumber
		}
		c.mut.Unlock()
		duration := time.Since(cmd.sendTime)
		c.stats.AddLatency(duration)
		// if c.conf.Benchmark {
		// 	c.data.Stats = append(c.data.Stats, &clientpb.CommandStats{
		// 		StartTime: timestamppb.New(cmd.sendTime),
		// 		Duration:  durationpb.New(duration),
		// 	})
		// }
	}
}
