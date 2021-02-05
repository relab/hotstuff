package main

import (
	"container/list"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
	"google.golang.org/protobuf/proto"
)

type cmdCache struct {
	mut           sync.Mutex
	batchSize     int
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
	marshaler     proto.MarshalOptions
	unmarshaler   proto.UnmarshalOptions
}

func newCmdCache(batchSize int) *cmdCache {
	return &cmdCache{
		batchSize:     batchSize,
		serialNumbers: make(map[uint32]uint64),
		marshaler:     proto.MarshalOptions{Deterministic: true},
		unmarshaler:   proto.UnmarshalOptions{DiscardUnknown: true},
	}
}

func (c *cmdCache) addCommand(cmd *client.Command) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if serialNo := c.serialNumbers[cmd.GetClientID()]; serialNo >= cmd.GetSequenceNumber() {
		// command is too old
		return
	}
	c.cache.PushBack(cmd)
}

func (c *cmdCache) GetCommand() *hotstuff.Command {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.cache.Len() == 0 {
		return nil
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
		return nil
	}

	cmd := hotstuff.Command(b)
	return &cmd
}

func (c *cmdCache) Accept(cmd hotstuff.Command) bool {
	batch := new(client.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
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
