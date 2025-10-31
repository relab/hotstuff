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
	// The goroutine can safely lock the mutex because c.cond.Wait() atomically
	// unlocks the mutex while waiting, allowing the goroutine to acquire it.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// Wait() has unlocked c.mut, so we can lock it here.
			c.mut.Lock()
			c.cond.Broadcast()
			c.mut.Unlock()
		case <-done:
		}
	}()

	for {
		// Try to extract a batch if we have enough commands.
		if c.len() >= c.batchSize {
			batch := c.extractBatch()
			if len(batch.Commands) > 0 {
				return batch, nil
			}
			// All commands were duplicates, continue waiting.
		}

		// Check if context is done before waiting.
		// This handles the case where context was canceled before we start waiting.
		if ctx.Err() != nil {
			// Return whatever commands we have, or error if none.
			batch := c.extractBatch()
			if len(batch.Commands) > 0 {
				return batch, nil
			}
			return nil, ctx.Err()
		}

		// Wait for more commands or context cancellation.
		// This atomically unlocks c.mut, waits for a signal, then re-locks c.mut.
		c.cond.Wait()

		// After waking up, check if context was canceled.
		// This is necessary because the wake-up might be due to context cancellation.
		if ctx.Err() != nil {
			// Return whatever commands we have, or error if none.
			batch := c.extractBatch()
			if len(batch.Commands) > 0 {
				return batch, nil
			}
			return nil, ctx.Err()
		}
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
