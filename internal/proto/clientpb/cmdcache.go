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

// Add adds a command to the cache and notifies waiting Get calls if a batch can be formed.
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
// It blocks until it can return a full batch, or until the context is done.
// If the context is done and there are commands available, it returns a partial batch.
// If the context is done and no commands are available, it returns an error.
// NOTE: the commands should be marked as proposed to avoid duplicates.
func (c *CommandCache) Get(ctx context.Context) (*Batch, error) {
	c.mut.Lock()
	defer c.mut.Unlock()

	// Start a goroutine to wake us up when context is done.
	// This goroutine is cleaned up when this function returns via defer.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.mut.Lock()
			c.cond.Broadcast()
			c.mut.Unlock()
		case <-done:
		}
	}()

	for {
		// Check if we should return a batch.
		// Return a full batch if we have enough commands, or a partial batch if context is done.
		if c.len() >= c.batchSize || ctx.Err() != nil {
			batch := c.extractBatch()
			if len(batch.Commands) > 0 {
				// We have commands, return them.
				return batch, nil
			}
			// No non-duplicate commands available.
			if ctx.Err() != nil {
				// Context is done and no commands, return error.
				return nil, ctx.Err()
			}
			// All commands were duplicates, wait for more.
		}

		// Wait for more commands or context cancellation.
		c.cond.Wait()
	}
}

// extractBatch extracts up to batchSize commands from the cache, skipping duplicates.
// Returns an empty batch if no non-duplicate commands are found.
// Callers must hold the lock.
func (c *CommandCache) extractBatch() *Batch {
	batch := new(Batch)

	for i := uint32(0); i < c.batchSize; i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		cmd := c.cache.Remove(elem).(*Command)
		if c.isDuplicate(cmd) {
			// command is too old, skip it
			i--
			continue
		}
		batch.Commands = append(batch.Commands, cmd)
	}

	return batch
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
