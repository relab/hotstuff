package cmdcache

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/protobuf/proto"
)

// CmdID is a unique identifier for a command
type CmdID struct {
	ClientID    uint32
	SequenceNum uint64
}

type Cache struct {
	logger logging.Logger

	mut           sync.Mutex
	c             chan struct{}
	batchSize     uint32
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
	marshaler     proto.MarshalOptions
	unmarshaler   proto.UnmarshalOptions
}

func New(
	logger logging.Logger,
	opts ...Option,
) *Cache {
	c := &Cache{
		logger: logger,

		c:             make(chan struct{}),
		batchSize:     1,
		serialNumbers: make(map[uint32]uint64),
		marshaler:     proto.MarshalOptions{Deterministic: true},
		unmarshaler:   proto.UnmarshalOptions{DiscardUnknown: true},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Cache) len() uint32 {
	return uint32(c.cache.Len())
}

func (c *Cache) Add(cmd *clientpb.Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
		// command is too old
		return
	}
	c.cache.PushBack(cmd)
	if c.len() >= c.batchSize {
		// notify Get that we are ready to send a new batch.
		select {
		case c.c <- struct{}{}:
		default:
		}
	}
}

// Get returns a batch of commands to propose.
func (c *Cache) Get(ctx context.Context) (cmd hotstuff.Command, ok bool) {
	batch := new(clientpb.Batch)

	c.mut.Lock()
awaitBatch:
	// wait until we can send a new batch.
	for c.len() <= c.batchSize {
		c.mut.Unlock()
		select {
		case <-c.c:
		case <-ctx.Done():
			return
		}
		c.mut.Lock()
	}

	// Get the batch. Note that we may not be able to fill the batch, but that should be fine as long as we can send
	// at least one command.
	for i := uint32(0); i < c.batchSize; i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		c.cache.Remove(elem)
		cmd := elem.Value.(*clientpb.Command)
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
			// command is too old
			i--
			continue
		}
		batch.Commands = append(batch.Commands, cmd)
	}

	// if we still got no (new) commands, try to wait again
	if len(batch.Commands) == 0 {
		goto awaitBatch
	}

	defer c.mut.Unlock()

	// otherwise, we should have at least one command
	b, err := c.marshaler.Marshal(batch)
	if err != nil {
		c.logger.Errorf("Failed to marshal batch: %v", err)
		return "", false
	}

	cmd = hotstuff.Command(b)
	return cmd, true
}

// Accept returns true if the batch is new.
func (c *Cache) Accept(cmd hotstuff.Command) bool {
	batch := new(clientpb.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
		c.logger.Errorf("Failed to unmarshal batch: %v", err)
		return false
	}

	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch.GetCommands() {
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
			// command is too old, can't accept
			return false
		}
	}

	return true
}

// Proposed updates the serial numbers such that we will not accept the given batch again.
func (c *Cache) Proposed(cmd hotstuff.Command) {
	batch := new(clientpb.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
		c.logger.Errorf("Failed to unmarshal batch: %v", err)
		return
	}

	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch.GetCommands() {
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo < cmd.GetSequenceNumber() {
			c.serialNumbers[cmd.GetClientID()] = cmd.GetSequenceNumber()
		}
	}
}
