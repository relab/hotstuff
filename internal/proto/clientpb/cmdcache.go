package clientpb

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

type Cache struct {
	mut           sync.Mutex
	c             chan struct{}
	batchSize     uint32
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
}

func New(
	opts ...Option,
) *Cache {
	c := &Cache{
		c:             make(chan struct{}),
		batchSize:     1,
		serialNumbers: make(map[uint32]uint64),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Cache) len() uint32 {
	return uint32(c.cache.Len())
}

func (c *Cache) add(cmd *Command) {
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
func (c *Cache) Get(ctx context.Context) (*Batch, error) {
	batch := new(Batch)

	c.mut.Lock()
awaitBatch:
	// wait until we can send a new batch.
	for c.len() < c.batchSize {
		c.mut.Unlock()
		select {
		case <-c.c:
		case <-ctx.Done():
			return nil, ctx.Err()
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

	c.mut.Unlock()

	return batch, nil
}

// Accept returns an error if the given command batch is too old to be accepted.
func (c *Cache) Accept(batch *Batch) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch.GetCommands() {
		// TODO(meling): Should this error out on the first old command?
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
			return fmt.Errorf("command too old")
		}
	}
	return nil
}

// Proposed updates the serial numbers such that we will not accept the given batch again.
func (c *Cache) Proposed(batch *Batch) {
	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch.GetCommands() {
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo < cmd.GetSequenceNumber() {
			c.serialNumbers[cmd.GetClientID()] = cmd.GetSequenceNumber()
		}
	}
}
