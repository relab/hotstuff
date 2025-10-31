// Package clientpb provides the service interface and command batcher for clients.
package clientpb

import (
	"container/list"
	"context"
	"slices"
	"sync"
)

type CommandCache struct {
	mut              sync.Mutex
	cond             *sync.Cond
	batchSize        uint32
	clientSeqNumbers map[uint32]uint64 // highest proposed sequence number per client ID
	cache            list.List
}

func NewCommandCache(batchSize uint32) *CommandCache {
	c := &CommandCache{
		batchSize:        batchSize,
		clientSeqNumbers: make(map[uint32]uint64),
	}
	c.cond = sync.NewCond(&c.mut)
	return c
}

func (c *CommandCache) len() uint32 {
	return uint32(c.cache.Len())
}

// isDuplicate returns true if the given command has already been processed.
// Callers must hold the lock to access the underlying clientSeqNumbers map.
func (c *CommandCache) isDuplicate(cmd *Command) bool {
	seqNum := c.clientSeqNumbers[cmd.GetClientID()]
	return seqNum >= cmd.GetSequenceNumber()
}

func (c *CommandCache) Add(cmd *Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.isDuplicate(cmd) {
		// command is too old
		return
	}
	c.cache.PushBack(cmd)
	if c.len() >= c.batchSize {
		// notify Get that we are ready to send a new batch.
		c.cond.Signal()
	}
}

// Get returns a batch of commands to propose.
// It blocks until it can return a batch of commands, or the context is done.
// If the context is done, it returns an error.
// NOTE: the commands should be marked as proposed to avoid duplicates.
func (c *CommandCache) Get(ctx context.Context) (*Batch, error) {
	batch := new(Batch)

	c.mut.Lock()
awaitBatch:
	// wait until we have enough commands for a batch.
	for c.len() < c.batchSize {
		// Check if context is already done before waiting
		select {
		case <-ctx.Done():
			c.mut.Unlock()
			return nil, ctx.Err()
		default:
		}

		// Wait for signal that commands are available.
		// We use a goroutine to handle context cancellation while waiting.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				c.mut.Lock()
				c.cond.Broadcast() // Wake up waiting Get() to check context
				c.mut.Unlock()
			case <-done:
			}
		}()

		c.cond.Wait() // Atomically unlock c.mut, wait, and re-lock c.mut
		close(done)

		// c.mut is locked again after Wait() returns
		// Check context again after waking up
		select {
		case <-ctx.Done():
			c.mut.Unlock()
			return nil, ctx.Err()
		default:
		}
		// Loop continues to check c.len() < c.batchSize again
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

// containsDuplicate returns true if the batch contains old commands already proposed.
// NOTE: This method is only used in the tests.
func (c *CommandCache) containsDuplicate(batch *Batch) bool {
	c.mut.Lock()
	defer c.mut.Unlock()

	return slices.ContainsFunc(batch.GetCommands(), c.isDuplicate)
}

// Proposed updates the sequence numbers such that we will not accept the given batch again.
func (c *CommandCache) Proposed(batch *Batch) {
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
