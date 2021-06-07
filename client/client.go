package client

import (
	"context"
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
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/grpc"
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

// Config contains config options for a client.
type Config struct {
	ID            consensus.ID
	TLS           bool
	MaxConcurrent uint32
	PayloadSize   uint32
	Input         io.ReadCloser
}

// Client is a hotstuff client.
type Client struct {
	id               consensus.ID
	logger           logging.Logger
	replicaConfig    *config.ReplicaConfig
	mgr              *clientpb.Manager
	gorumsConfig     *clientpb.Configuration
	payloadSize      uint32
	mut              sync.Mutex
	highestCommitted uint64 // highest sequence number acknowledged by the replicas
	pendingCmds      chan pendingCmd
	cancel           context.CancelFunc
	done             chan struct{}
	reader           io.ReadCloser
	stats            benchmark.Stats         // records latency and throughput
	data             *clientpb.BenchmarkData // stores time and duration for each command
}

// New returns a new Client.
func New(conf Config) (client *Client) {
	logger := logging.New(fmt.Sprintf("cli%d", conf.ID))

	client = &Client{
		id:               conf.ID,
		logger:           logger,
		pendingCmds:      make(chan pendingCmd, conf.MaxConcurrent),
		highestCommitted: 1,
		done:             make(chan struct{}),
		data:             &clientpb.BenchmarkData{},
	}

	return client
}

// Connect connects the client to the replicas.
func (c *Client) Connect(replicaConfig *config.ReplicaConfig, opts ...gorums.ManagerOption) (err error) {
	nodes := make(map[string]uint32, len(replicaConfig.Replicas))
	for _, r := range replicaConfig.Replicas {
		nodes[r.Address] = uint32(r.ID)
	}

	grpcOpts := []grpc.DialOption{grpc.WithBlock()}

	if replicaConfig.Creds != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(replicaConfig.Creds))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	c.mgr = clientpb.NewManager(opts...)

	c.gorumsConfig, err = c.mgr.NewConfiguration(&qspec{faulty: consensus.NumFaulty(len(replicaConfig.Replicas))}, gorums.WithNodeMap(nodes))
	if err != nil {
		c.mgr.Close()
		return err
	}
	return nil
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

func (c *Client) Start() {
	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())
	go c.Run(ctx)
	close(c.done)
}

func (c *Client) Stop() {
	c.cancel()
	<-c.done
}

func (c *Client) close() {
	c.mgr.Close()
	err := c.reader.Close()
	if err != nil {
		c.logger.Warn("Failed to close reader: ", err)
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

		data := make([]byte, c.payloadSize)
		n, err := c.reader.Read(data)
		if err != nil && err != io.EOF {
			// if we get an error other than EOF
			return err
		} else if err == io.EOF && n == 0 && lastCommand > num {
			lastCommand = num
			c.logger.Info("Reached end of file. Sending empty commands until last command is executed...")
		}

		cmd := &clientpb.Command{
			ClientID:       uint32(c.id),
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
