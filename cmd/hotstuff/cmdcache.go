package main

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/client"
	"google.golang.org/protobuf/proto"
)

type cmdCache struct {
	mut           sync.Mutex
	mod           *consensus.Modules
	c             chan struct{}
	batchSize     int
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
	marshaler     proto.MarshalOptions
	unmarshaler   proto.UnmarshalOptions
}

func newCmdCache(batchSize int) *cmdCache {
	return &cmdCache{
		c:             make(chan struct{}),
		batchSize:     batchSize,
		serialNumbers: make(map[uint32]uint64),
		marshaler:     proto.MarshalOptions{Deterministic: true},
		unmarshaler:   proto.UnmarshalOptions{DiscardUnknown: true},
	}
}

// InitModule gives the module a reference to the HotStuff object.
func (c *cmdCache) InitModule(hs *consensus.Modules, _ *consensus.OptionsBuilder) {
	c.mod = hs
}

func (c *cmdCache) addCommand(cmd *client.Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
		// command is too old
		return
	}
	c.cache.PushBack(cmd)
	if c.cache.Len() >= c.batchSize {
		// notify Get that we are ready to send a new batch.
		select {
		case c.c <- struct{}{}:
		default:
		}
	}
}

// Get returns a batch of commands to propose.
func (c *cmdCache) Get(ctx context.Context) (cmd consensus.Command, ok bool) {
	batch := new(client.Batch)

	c.mut.Lock()
awaitBatch:
	// wait until we can send a new batch.
	for c.cache.Len() <= c.batchSize {
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
	for i := 0; i < c.batchSize; i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		c.cache.Remove(elem)
		cmd := elem.Value.(*client.Command)
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
		c.mod.Logger().Errorf("Failed to marshal batch: %v", err)
		return "", false
	}

	cmd = consensus.Command(b)
	return cmd, true
}

// Accept returns true if the replica can accept the batch.
func (c *cmdCache) Accept(cmd consensus.Command) bool {
	batch := new(client.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
		c.mod.Logger().Errorf("Failed to unmarshal batch: %v", err)
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
func (c *cmdCache) Proposed(cmd consensus.Command) {
	batch := new(client.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
		c.mod.Logger().Errorf("Failed to unmarshal batch: %v", err)
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

var _ consensus.Acceptor = (*cmdCache)(nil)
