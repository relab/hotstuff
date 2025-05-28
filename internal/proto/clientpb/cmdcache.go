package clientpb

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"google.golang.org/protobuf/proto"
)

type Cache struct {
	mut           sync.Mutex
	c             chan struct{}
	batchSize     uint32
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
	marshaler     proto.MarshalOptions
	unmarshaler   proto.UnmarshalOptions
}

func New(
	opts ...Option,
) *Cache {
	c := &Cache{
		c:             make(chan struct{}),
		batchSize:     1,
		serialNumbers: make(map[uint32]uint64),
		marshaler:     proto.MarshalOptions{Deterministic: true},
		unmarshaler:   proto.UnmarshalOptions{DiscardUnknown: true, AllowPartial: true},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Cache) len() uint32 {
	return uint32(c.cache.Len())
}

func (c *Cache) Add(cmd *Command) {
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
// It blocks until it can return a batch of commands, or the context is done.
// If the context is done, it returns an error.
func (c *Cache) Get(ctx context.Context) (hotstuff.Command, error) {
	batch := new(Batch)

	c.mut.Lock()
awaitBatch:
	// wait until we can send a new batch.
	for c.len() < c.batchSize {
		c.mut.Unlock()
		select {
		case <-c.c:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		c.mut.Lock()
	}

	// Get the batch. Note that we may not be able to fill the batch,
	// but that's okay as long as we can send at least one command.
	for i := uint32(0); i < c.batchSize; i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		cmd := c.cache.Remove(elem).(*Command)
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
		return "", fmt.Errorf("failed to marshal batch: %w", err)
	}

	return hotstuff.Command(b), nil
}

// Accept returns an error if the given command batch is too old to be accepted.
func (c *Cache) Accept(cmd hotstuff.Command) error {
	batch, err := c.GetCommands(cmd)
	if err != nil {
		return err
	}

	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch {
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
			return fmt.Errorf("command too old")
		}
	}
	return nil
}

// Proposed updates the serial numbers such that we will not accept the given batch again.
func (c *Cache) Proposed(cmd hotstuff.Command) error {
	batch, err := c.GetCommands(cmd)
	if err != nil {
		return err
	}

	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch {
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo < cmd.GetSequenceNumber() {
			c.serialNumbers[cmd.GetClientID()] = cmd.GetSequenceNumber()
		}
	}
	return nil
}

// GetCommands unmarshals the given command returns its batch of commands.
func (c *Cache) GetCommands(cmd hotstuff.Command) ([]*Command, error) {
	batch := new(Batch)
	if err := c.unmarshaler.Unmarshal([]byte(cmd), batch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch: %w", err)
	}
	return batch.GetCommands(), nil
}
