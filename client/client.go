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
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	ID               hotstuff.ID
	TLS              bool
	RootCAs          *x509.CertPool
	MaxConcurrent    uint32
	PayloadSize      uint32
	Input            io.ReadCloser
	ManagerOptions   []gorums.ManagerOption
	RateLimit        float64       // initial rate limit
	RateStep         float64       // rate limit step up
	RateStepInterval time.Duration // step up interval
	Replicas         []backend.ReplicaInfo
}

// Client is a hotstuff client.
type Client struct {
	mut                sync.Mutex
	id                 hotstuff.ID
	mods               *modules.Modules
	nodeMgr            *clientpb.Manager
	activeNodeConfig   *clientpb.Configuration
	payloadSize        uint32
	highestCommitted   uint64 // highest sequence number acknowledged by the replicas
	pendingCmds        chan pendingCmd
	cancel             context.CancelFunc
	done               chan struct{}
	reader             io.ReadCloser
	limiter            *rate.Limiter
	stepUp             float64
	stepUpInterval     time.Duration
	orchestratorConfig *clientpb.Configuration
	replicas           []backend.ReplicaInfo
}

// New returns a new Client.
func New(conf Config, builder modules.Builder) (client *Client) {
	builder.Register(logging.New("cli" + strconv.Itoa(int(conf.ID))))
	mods := builder.Build()

	client = &Client{
		id:               conf.ID,
		mods:             mods,
		pendingCmds:      make(chan pendingCmd, conf.MaxConcurrent),
		highestCommitted: 1,
		done:             make(chan struct{}),
		reader:           conf.Input,
		payloadSize:      conf.PayloadSize,
		limiter:          rate.NewLimiter(rate.Limit(conf.RateLimit), 1),
		stepUp:           conf.RateStep,
		stepUpInterval:   conf.RateStepInterval,
		replicas:         conf.Replicas,
	}

	grpcOpts := []grpc.DialOption{grpc.WithBlock()}

	var creds credentials.TransportCredentials
	if conf.TLS {
		creds = credentials.NewClientTLSFromCert(conf.RootCAs, "")
	} else {
		creds = insecure.NewCredentials()
	}
	grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))

	opts := conf.ManagerOptions
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	client.nodeMgr = clientpb.NewManager(opts...)

	return client
}

// Connect connects the client to the replicas.
func (c *Client) Connect() (err error) {
	activeNodes := make(map[string]uint32)
	orchestrators := make(map[string]uint32)
	for _, r := range c.replicas {
		if r.NodeState == hotstuff.Active {
			activeNodes[r.Address] = uint32(r.ID)
		} else if r.NodeState == hotstuff.Orchestrator {
			orchestrators[r.Address] = uint32(r.ID)
		}
	}
	c.activeNodeConfig, err = c.nodeMgr.NewConfiguration(&qspec{faulty: hotstuff.NumFaulty(len(activeNodes))}, gorums.WithNodeMap(activeNodes))
	if err != nil {
		c.nodeMgr.Close()
		return err
	}
	c.orchestratorConfig, err = c.nodeMgr.NewConfiguration(&qspec{faulty: hotstuff.NumFaulty(len(orchestrators))}, gorums.WithNodeMap(orchestrators))
	if err != nil {
		c.nodeMgr.Close()
		return err
	}
	return nil
}

// Run runs the client until the context is closed.
func (c *Client) Run(ctx context.Context) {
	eventLoopDone := make(chan struct{})
	go func() {
		c.mods.EventLoop().Run(ctx)
		close(eventLoopDone)
	}()
	c.mods.Logger().Info("Starting to send commands")

	commandStatsChan := make(chan struct{ executed, failed int })
	// start the command handler
	go func() {
		executed, failed := c.handleCommands(ctx)
		commandStatsChan <- struct {
			executed int
			failed   int
		}{executed, failed}
	}()

	// TODO(hanish) Make it readable from the config/const
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.performReconfiguration(ctx)
		}
	}()

	err := c.sendCommands(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		c.mods.Logger().Panicf("Failed to send commands: %v", err)
	}
	c.close()

	stats := <-commandStatsChan
	c.mods.Logger().Infof("Done sending commands (executed: %d, failed: %d)", stats.executed, stats.failed)
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
	c.nodeMgr.Close()
	err := c.reader.Close()
	if err != nil {
		c.mods.Logger().Warn("Failed to close reader: ", err)
	}
}

func (c *Client) findLeastIdNode(NodeType hotstuff.ReplicaState) uint32 {
	nodeIDList := make([]uint32, 0)
	for _, replica := range c.replicas {
		if replica.NodeState == NodeType {
			nodeIDList = append(nodeIDList, uint32(replica.ID))
		}
	}
	sort.Slice(nodeIDList, func(i int, j int) bool {
		return nodeIDList[i] < nodeIDList[j]
	})
	return nodeIDList[0]
}

// performReconfiguration issues the reconfiguration request to the orchestrators.
func (c *Client) performReconfiguration(ctx context.Context) error {
	// Move least active replica to read mode and move least learn node to active mode
	// this is the policy but can be changed in future.
	leastActiveNode := c.findLeastIdNode(hotstuff.Active)
	leastLearnNode := c.findLeastIdNode(hotstuff.Learn)
	leastReadNode := c.findLeastIdNode(hotstuff.Read)
	replicas := make(map[uint32]orchestrationpb.ReplicaState)
	for _, replica := range c.replicas {
		switch uint32(replica.ID) {
		case leastActiveNode:
			replica.NodeState = hotstuff.Read
		case leastLearnNode:
			replica.NodeState = hotstuff.Active
		case leastReadNode:
			replica.NodeState = hotstuff.Learn
		}
		replicas[uint32(replica.ID)] = orchestrationpb.ReplicaState(replica.NodeState)
	}
	req := orchestrationpb.ReconfigurationRequest{Replicas: replicas}
	c.orchestratorConfig.ReconfigureRequest(ctx, &req, gorums.WithNoSendWaiting())
	return nil
}

func (q qspec) ReconfigureQF(in *orchestrationpb.ReconfigurationRequest,
	replies map[uint32]*hotstuffpb.SyncInfo) (*hotstuffpb.SyncInfo, bool) {

	return nil, false
}

func (c *Client) sendCommands(ctx context.Context) error {
	var (
		num         uint64 = 1
		lastCommand uint64 = math.MaxUint64
		lastStep           = time.Now()
	)

loop:
	for {
		if ctx.Err() != nil {
			break
		}

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
			c.mods.Logger().Info("Reached end of file. Sending empty commands until last command is executed...")
		}

		cmd := &clientpb.Command{
			ClientID:       uint32(c.id),
			SequenceNumber: num,
			Data:           data[:n],
		}

		promise := c.activeNodeConfig.ExecCommand(ctx, cmd)

		num++
		select {
		case c.pendingCmds <- pendingCmd{sequenceNumber: num, sendTime: time.Now(), promise: promise}:
		case <-ctx.Done():
			break loop
		}

		if num%100 == 0 {
			c.mods.Logger().Infof("%d commands sent", num)
		}

	}
	return nil
}

// handleCommands will get pending commands from the pendingCmds channel and then
// handle them as they become acknowledged by the replicas. We expect the commands to be
// acknowledged in the order that they were sent.
func (c *Client) handleCommands(ctx context.Context) (executed, failed int) {
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
				c.mods.Logger().Debugf("Did not get enough replies for command: %v\n", err)
				failed++
			}
		} else {
			executed++
		}
		c.mut.Lock()
		if cmd.sequenceNumber > c.highestCommitted {
			c.highestCommitted = cmd.sequenceNumber
		}
		c.mut.Unlock()

		duration := time.Since(cmd.sendTime)
		c.mods.EventLoop().AddEvent(LatencyMeasurementEvent{Latency: duration})
	}
}

// LatencyMeasurementEvent represents a single latency measurement.
type LatencyMeasurementEvent struct {
	Latency time.Duration
}
