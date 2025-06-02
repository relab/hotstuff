package clientpb

import (
	"container/list"
	"context"
	"slices"
	"sync"
)

type Cache struct {
	mut              sync.Mutex
	c                chan struct{}
	batchSize        uint32
	clientSeqNumbers map[uint32]uint64 // highest proposed sequence number per client ID
	cache            list.List
}

func New(
	opts ...Option,
) *Cache {
	c := &Cache{
		c:                make(chan struct{}),
		batchSize:        1,
		clientSeqNumbers: make(map[uint32]uint64),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Cache) len() uint32 {
	return uint32(c.cache.Len())
}

func (c *Cache) isDuplicate(cmd *Command) bool {
	seqNum := c.clientSeqNumbers[cmd.GetClientID()]
	return seqNum >= cmd.GetSequenceNumber()
}

func (c *Cache) add(cmd *Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.isDuplicate(cmd) {
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
		if c.isDuplicate(cmd) {
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

// ContainsDuplicate returns true if the batch contains old commands already proposed.
func (c *Cache) ContainsDuplicate(batch *Batch) bool {
	c.mut.Lock()
	defer c.mut.Unlock()

	return slices.ContainsFunc(batch.GetCommands(), c.isDuplicate)
}

// Proposed updates the sequence numbers such that we will not accept the given batch again.
func (c *Cache) Proposed(batch *Batch) {
	c.mut.Lock()
	defer c.mut.Unlock()

	for _, cmd := range batch.GetCommands() {
		if !c.isDuplicate(cmd) {
			// the command is new (not a duplicate); we update the highest
			// sequence number for the client that sent the command
			c.clientSeqNumbers[cmd.GetClientID()] = cmd.GetSequenceNumber()
		}
	}
}
