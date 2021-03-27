package main

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"google.golang.org/protobuf/proto"
)

type cmdCache struct {
	mut           sync.Mutex
	mod           *hotstuff.HotStuff
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
func (c *cmdCache) InitModule(hs *hotstuff.HotStuff) {
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

func (c *cmdCache) Get(ctx context.Context) (cmd hotstuff.Command, ok bool) {
	c.mut.Lock()
	defer c.mut.Unlock()

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

	batch := new(client.Batch)

	for i := 0; i < c.batchSize; i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		c.cache.Remove(elem)
		cmd := elem.Value.(*client.Command)
		if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
			// command is too old, can't propose
			continue
		}
		batch.Commands = append(batch.Commands, cmd)
	}

	b, err := c.marshaler.Marshal(batch)
	if err != nil {
		c.mod.Logger().Errorf("Failed to marshal batch: %v", err)
		return "", false
	}

	cmd = hotstuff.Command(b)
	return cmd, true
}

func (c *cmdCache) Accept(cmd hotstuff.Command) bool {
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
		c.serialNumbers[cmd.GetClientID()] = cmd.GetSequenceNumber()
	}

	return true
}
