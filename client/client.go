// Package client implements a simple client for testing HotStuff.
// The client reads data from an input stream and sends the data in commands to a HotStuff replica.
// The client waits for replies from f+1 replicas before it considers a command to be executed.
package client

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ID is the identifier for a client.
type ID uint32

type qspec struct {
	faulty int
}

func (q *qspec) ExecCommandQF(_ *clientpb.Command, signatures map[uint32]*emptypb.Empty) (*emptypb.Empty, bool) {
	if len(signatures) < q.faulty+1 {
		return nil, false
	}
	return &emptypb.Empty{}, true
}

type pendingCmd struct {
	sequenceNumber uint64
	sendTime       time.Time
	cancelCtx      context.CancelFunc
}

// Config contains config options for a client.
type Config struct {
	TLS              bool
	RootCAs          *x509.CertPool
	MaxConcurrent    uint32
	PayloadSize      uint32
	Input            io.ReadCloser
	ManagerOptions   []gorums.ManagerOption
	RateLimit        float64       // initial rate limit
	RateStep         float64       // rate limit step up
	RateStepInterval time.Duration // step up interval
	Timeout          time.Duration
}

// Client is a hotstuff client.
type Client struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	id        ID

	mut              sync.Mutex
	mgr              *clientpb.Manager
	gorumsConfig     *clientpb.Configuration
	payloadSize      uint32
	highestCommitted uint64 // highest sequence number acknowledged by the replicas
	pendingCmds      chan pendingCmd
	cancel           context.CancelFunc
	done             chan struct{}
	reader           io.ReadCloser
	limiter          *rate.Limiter
	stepUp           float64
	stepUpInterval   time.Duration
	timeout          time.Duration
}

// New returns a new Client.
func New(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	id ID,
	conf Config,
) (client *Client) {
	client = &Client{
		eventLoop: eventLoop,
		logger:    logger,
		id:        id,

		pendingCmds:      make(chan pendingCmd, conf.MaxConcurrent),
		highestCommitted: 1,
		done:             make(chan struct{}),
		reader:           conf.Input,
		payloadSize:      conf.PayloadSize,
		limiter:          rate.NewLimiter(rate.Limit(conf.RateLimit), 1),
		stepUp:           conf.RateStep,
		stepUpInterval:   conf.RateStepInterval,
		timeout:          conf.Timeout,
	}

	var creds credentials.TransportCredentials
	if conf.TLS {
		creds = credentials.NewClientTLSFromCert(conf.RootCAs, "")
	} else {
		creds = insecure.NewCredentials()
	}

	mgrOpts := append(conf.ManagerOptions, gorums.WithGrpcDialOptions(grpc.WithTransportCredentials(creds)))

	client.mgr = clientpb.NewManager(mgrOpts...)

	return client
}

// Connect connects the client to the replicas.
func (c *Client) Connect(replicas []hotstuff.ReplicaInfo) (err error) {
	nodes := make(map[string]uint32, len(replicas))
	for _, r := range replicas {
		nodes[r.Address] = uint32(r.ID)
	}
	c.gorumsConfig, err = c.mgr.NewConfiguration(&qspec{faulty: hotstuff.NumFaulty(len(replicas))}, gorums.WithNodeMap(nodes))
	if err != nil {
		c.mgr.Close()
		return err
	}
	return nil
}

// Run runs the client until the context is closed.
func (c *Client) Run(ctx context.Context) {
	type stats struct {
		executed int
		failed   int
		timeout  int
	}

	eventLoopDone := make(chan struct{})
	go func() {
		c.eventLoop.Run(ctx)
		close(eventLoopDone)
	}()
	c.logger.Info("Starting to send commands")

	commandStatsChan := make(chan stats)
	// start the command handler
	go func() {
		executed, failed, timeout := c.handleCommands(ctx)
		commandStatsChan <- stats{executed, failed, timeout}
	}()

	err := c.sendCommands(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		c.logger.Panicf("Failed to send commands: %v", err)
	}
	c.close()

	commandStats := <-commandStatsChan
	c.logger.Infof(
		"Done sending commands (executed: %d, failed: %d, timeouts: %d)",
		commandStats.executed, commandStats.failed, commandStats.timeout,
	)
	<-eventLoopDone
	close(c.done)
}

// Start starts the client.
func (c *Client) Start() {
	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())
	go c.Run(ctx)
}

// Stop stops the client.
func (c *Client) Stop() {
	c.cancel()
	<-c.done
}

func (c *Client) close() {
	// Signal the command handler to stop fetching statuses before closing the manager.
	close(c.pendingCmds)
	c.mgr.Close()
	err := c.reader.Close()
	if err != nil {
		c.logger.Warn("Failed to close reader: ", err)
	}
}

func (c *Client) sendCommands(ctx context.Context) error {
	var (
		num         uint64 = 1
		lastCommand uint64 = math.MaxUint64
		lastStep           = time.Now()
		nextLogTime        = time.Now().Add(time.Second)
	)

loop:
	for ctx.Err() == nil {

		// step up the rate limiter
		now := time.Now()
		if now.Sub(lastStep) > c.stepUpInterval {
			c.limiter.SetLimit(c.limiter.Limit() + rate.Limit(c.stepUp))
			lastStep = now
		}

		err := c.limiter.Wait(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
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

		ctx, cancel := context.WithTimeout(ctx, c.timeout)
		c.gorumsConfig.ExecCommand(ctx, cmd)
		pending := pendingCmd{sequenceNumber: num, sendTime: time.Now(), cancelCtx: cancel}

		num++
		select {
		case c.pendingCmds <- pending:
		case <-ctx.Done():
			break loop
		}

		if time.Now().After(nextLogTime) {
			c.logger.Infof("%d commands sent so far", num)
			nextLogTime = time.Now().Add(time.Second)
		}

	}
	return nil
}

func (c *Client) fetchCommandStatus(sequenceNumber uint64) hotstuff.CommandStatus {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(c.timeout)

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cmd := &clientpb.Command{
				ClientID:       uint32(c.id),
				SequenceNumber: sequenceNumber,
			}
			nodes := c.gorumsConfig.Nodes()
			if len(nodes) == 0 {
				c.logger.Error("No nodes available in gorums config")
				cancel()
				continue
			}
			node := nodes[rand.Intn(len(nodes))]
			response, err := node.CommandStatus(ctx, cmd)
			cancel()

			if err != nil {
				c.logger.Errorf("Failed to fetch command status (client: %d, sequence: %d): %v", c.id, sequenceNumber, err)
				// If the node/manager was closed, stop trying and return UNKNOWN.
				if strings.Contains(err.Error(), "node closed") {
					return hotstuff.UNKNOWN
				}
				continue
			}
			if response == nil || response.Command == nil {
				c.logger.Errorf("Invalid response received when fetching command status (client: %d, sequence: %d)", c.id, sequenceNumber)
				continue
			}
			c.logger.Infof("Fetched command status (client: %d, sequence: %d, status: %d)", c.id, sequenceNumber, response.Status)
			status := hotstuff.CommandStatus(response.Status)
			if status == hotstuff.COMMITTED || status == hotstuff.EXECUTED || status == hotstuff.FAILED {
				return status
			}
		case <-timeout:
			return hotstuff.UNKNOWN
		}
	}
}

// handleCommands will get pending commands from the pendingCmds channel and then
// handle them as they become acknowledged by the replicas. We expect the commands to be
// acknowledged in the order that they were sent.
func (c *Client) handleCommands(ctx context.Context) (executed, failed, timeout int) {
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
		response := c.fetchCommandStatus(cmd.sequenceNumber)

		switch response {
		case hotstuff.UNKNOWN:
			c.logger.Infof("Command timed out (client: %d, sequence: %d)", c.id, cmd.sequenceNumber)
			timeout++
		case hotstuff.FAILED:
			c.logger.Infof("Command failed (client: %d, sequence: %d)", c.id, cmd.sequenceNumber)
			failed++
		default:
			c.logger.Infof("Command executed (client: %d, sequence: %d)", c.id, cmd.sequenceNumber)
			executed++
		}
		c.mut.Lock()
		if cmd.sequenceNumber > c.highestCommitted {
			c.highestCommitted = cmd.sequenceNumber
		}
		c.mut.Unlock()
		duration := time.Since(cmd.sendTime)
		c.eventLoop.AddEvent(LatencyMeasurementEvent{Latency: duration})
	}
}

// LatencyMeasurementEvent represents a single latency measurement.
type LatencyMeasurementEvent struct {
	Latency time.Duration
}
