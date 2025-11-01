// Package clientpb provides the service interface and command batcher for clients.
package clientpb

import (
	"context"
	"slices"
	"sync"
)

type CommandCache struct {
	mut              sync.Mutex
	batchSize        uint32
	clientSeqNumbers map[uint32]uint64 // highest proposed sequence number per client ID
	cache            []*Command
	ready            chan struct{} // signals when a batch is ready
}

func NewCommandCache(batchSize uint32) *CommandCache {
	return &CommandCache{
		batchSize:        batchSize,
		clientSeqNumbers: make(map[uint32]uint64),
		cache:            make([]*Command, 0),
		ready:            make(chan struct{}, 1), // buffered to avoid blocking Add
	}
}

// Add adds a command to the cache and notifies waiting Get calls if a batch can be formed.
func (c *CommandCache) Add(cmd *Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.isDuplicate(cmd) {
		// command is too old
		return
	}
	c.cache = append(c.cache, cmd)
	if c.hasFullBatch() {
		c.signalReady()
	}
}

// Get returns a batch of commands to propose.
// It blocks until it can return a batch of commands, or the context is done.
// If the context is done, it returns an error.
// NOTE: the commands should be marked as proposed to avoid duplicates.
func (c *CommandCache) Get(ctx context.Context) (*Batch, error) {
	for {
		// Wait for either a batch to be ready or context cancellation
		select {
		case <-c.ready:
			// A batch might be ready
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		// Check if we have enough commands for a batch
		c.mut.Lock()
		if !c.hasFullBatch() {
			// False alarm: not enough commands yet (race condition or duplicates filtered)
			c.mut.Unlock()
			continue
		}

		// Try to extract a full batch, filtering out duplicates
		if batch := c.tryExtractBatch(); batch != nil {
			// We have a full batch! Signal if more batches are ready in cache
			if c.hasFullBatch() {
				c.signalReady()
			}

			c.mut.Unlock()
			return batch, nil
		}

		// We don't have a full batch yet. Keep the cache intact and wait for more commands
		c.mut.Unlock()
	}
}

// hasFullBatch returns true if the cache has enough commands for at least one full batch.
// Callers must hold the lock.
func (c *CommandCache) hasFullBatch() bool {
	return uint32(len(c.cache)) >= c.batchSize
}

// signalReady sends a signal on the ready channel if it's not already signaled.
// This is a non-blocking operation that notifies waiting Get() calls.
// Callers must hold the lock.
func (c *CommandCache) signalReady() {
	select {
	case c.ready <- struct{}{}:
	default:
		// channel already has a signal, no need to add another
	}
}

// tryExtractBatch attempts to extract a full batch from the cache, filtering out duplicates.
// If successful, removes the examined commands from the cache and returns the batch.
// Returns nil if a full batch cannot be formed from available commands (cache is left unchanged).
// Callers must hold the lock.
func (c *CommandCache) tryExtractBatch() *Batch {
	batch := &Batch{}
	extracted := 0 // track how many commands we've examined in cache

	for !batch.isFull(c.batchSize) && extracted < len(c.cache) {
		cmd := c.cache[extracted]
		extracted++
		if c.isDuplicate(cmd) {
			// command is too old, skip it
			continue
		}
		batch.Commands = append(batch.Commands, cmd)
	}

	if batch.isFull(c.batchSize) {
		// We have a full batch! Remove the examined commands from cache
		c.cache = c.cache[extracted:]
		return batch
	}
	return nil
}

// isDuplicate returns true if the given command has already been processed.
// Callers must hold the lock.
func (c *CommandCache) isDuplicate(cmd *Command) bool {
	seqNum := c.clientSeqNumbers[cmd.GetClientID()]
	return seqNum >= cmd.GetSequenceNumber()
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
