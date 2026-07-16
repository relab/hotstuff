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
	"slices"
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
)

// ID is the identifier for a client.
type ID uint32

type qspec struct {
	faulty int
}

// leastOverlapSet returns the set of uint64 values that appear in all slices.
func leastOverlapSet(slices [][]uint64) []uint64 {
	if len(slices) == 0 {
		return []uint64{}
	}

	// Count occurrences of each value across all slices
	occurrences := make(map[uint64]int)
	for _, slice := range slices {
		seen := make(map[uint64]bool)
		for _, val := range slice {
			if !seen[val] {
				occurrences[val]++
				seen[val] = true
			}
		}
	}

	// Find values that appear in all slices
	result := []uint64{}
	numSlices := len(slices)
	for val, count := range occurrences {
		if count == numSlices {
			result = append(result, val)
		}
	}

	return result
}

func (q *qspec) CommandStatusQF(command *clientpb.Command, replies map[uint32]*clientpb.CommandStatusResponse) (*clientpb.CommandStatusResponse, bool) {
	if len(replies) < q.faulty+1 {
		return nil, false
	}
	successfulHighestCmds := make([]uint64, 0, len(replies))
	commandCount := make(map[uint64]int)
	failedCommandIdSets := make([][]uint64, 0, len(replies))
	for _, reply := range replies {
		successfulHighestCmds = append(successfulHighestCmds, reply.GetHighestSequenceNumber())
		_, ok := commandCount[reply.GetHighestSequenceNumber()]
		if !ok {
			commandCount[reply.GetHighestSequenceNumber()] = 1
			continue
		}
		commandCount[reply.GetHighestSequenceNumber()]++
		failedCommandIdSets = append(failedCommandIdSets, reply.GetFailedSequenceNumbers())
	}
	leastOverlapFailedCmds := leastOverlapSet(failedCommandIdSets)
	slices.Sort(successfulHighestCmds)
	slices.Reverse(successfulHighestCmds)
	for _, cmd := range successfulHighestCmds {
		if commandCount[cmd] >= q.faulty+1 {
			return &clientpb.CommandStatusResponse{
				HighestSequenceNumber: cmd,
				FailedSequenceNumbers: leastOverlapFailedCmds,
			}, true
		}
	}
	return nil, false
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

	mut                sync.Mutex
	mgr                *clientpb.Manager
	gorumsConfig       *clientpb.Configuration
	payloadSize        uint32
	highestCommitted   uint64 // highest sequence number acknowledged by the replicas
	cancel             context.CancelFunc
	done               chan struct{}
	reader             io.ReadCloser
	limiter            *rate.Limiter
	stepUp             float64
	stepUpInterval     time.Duration
	timeout            time.Duration
	failedCommandIdSet *BitSet
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

		highestCommitted:   1,
		done:               make(chan struct{}),
		reader:             conf.Input,
		payloadSize:        conf.PayloadSize,
		limiter:            rate.NewLimiter(rate.Limit(conf.RateLimit), 1),
		stepUp:             conf.RateStep,
		stepUpInterval:     conf.RateStepInterval,
		timeout:            conf.Timeout,
		failedCommandIdSet: NewBitSet(5000000), // assuming max 5 million commands for now
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
		c.logger.Error("unable to create the configuration in client")
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
		c.gorumsConfig.ExecCommand(context.Background(), cmd)
		num++
		if time.Now().After(nextLogTime) {
			c.logger.Infof("%d commands sent so far", num)
			nextLogTime = time.Now().Add(time.Second)
		}
	}
	return nil
}

// handleCommands will get pending commands from the pendingCmds channel and then
// handle them as they become acknowledged by the replicas. We expect the commands to be
// acknowledged in the order that they were sent.
func (c *Client) handleCommands(ctx context.Context) (executed, failed, timeout int) {
	for {
		statusRefresher := time.NewTicker(100 * time.Millisecond)
		select {
		case <-statusRefresher.C:
			commandStatus, err := c.gorumsConfig.CommandStatus(ctx, &clientpb.Command{
				ClientID: uint32(c.id),
			})
			if err != nil {
				c.logger.Error("Failed to get command status: ", err)
				continue
			}
			c.mut.Lock()
			if c.highestCommitted < commandStatus.HighestSequenceNumber {
				c.highestCommitted = commandStatus.HighestSequenceNumber
			}
			for _, failedSeqNum := range commandStatus.FailedSequenceNumbers {
				c.failedCommandIdSet.Add(failedSeqNum)
			}
			failed = c.failedCommandIdSet.Count()
			executed = int(c.highestCommitted) - failed
			timeout = 0
			c.mut.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// LatencyMeasurementEvent represents a single latency measurement.
type LatencyMeasurementEvent struct {
	Latency time.Duration
}

// BitSet is a space-efficient set for uint64 values
type BitSet struct {
	mut  sync.Mutex
	bits []uint64
}

func NewBitSet(maxVal uint64) *BitSet {
	size := (maxVal / 64) + 1
	return &BitSet{
		bits: make([]uint64, size),
	}
}

func (bs *BitSet) Add(val uint64) {
	bs.mut.Lock()
	defer bs.mut.Unlock()

	index := val / 64
	offset := val % 64
	if index < uint64(len(bs.bits)) {
		bs.bits[index] |= (1 << offset)
	}
}

func (bs *BitSet) Contains(val uint64) bool {
	bs.mut.Lock()
	defer bs.mut.Unlock()

	index := val / 64
	offset := val % 64
	if index < uint64(len(bs.bits)) {
		return (bs.bits[index] & (1 << offset)) != 0
	}
	return false
}

func (bs *BitSet) Count() int {
	bs.mut.Lock()
	defer bs.mut.Unlock()

	count := 0
	for _, word := range bs.bits {
		count += popcount(word)
	}
	return count
}

func popcount(x uint64) int {
	count := 0
	for x != 0 {
		x &= x - 1
		count++
	}
	return count
}
