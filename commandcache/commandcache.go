package commandcache

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"google.golang.org/protobuf/proto"
)

type cmdCache struct {
	mut           sync.Mutex
	mods          *consensus.Modules
	c             chan struct{}
	batchSize     int
	serialNumbers map[uint32]uint64 // highest proposed serial number per client ID
	cache         list.List
	marshaler     proto.MarshalOptions
	unmarshaler   proto.UnmarshalOptions
}

func New(batchSize int) *cmdCache {
	return &cmdCache{
		c:             make(chan struct{}),
		batchSize:     batchSize,
		serialNumbers: make(map[uint32]uint64),
		marshaler:     proto.MarshalOptions{Deterministic: true},
		unmarshaler:   proto.UnmarshalOptions{DiscardUnknown: true},
	}
}

// InitModule gives the module access to the other modules.
func (c *cmdCache) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	c.mods = mods
}

func (c *cmdCache) AddCommand(cmd *clientpb.Command) {
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

// getBatch: fetches the batch, available for processing
func (c *cmdCache) getBatch(ctx context.Context) (batch *clientpb.Batch, ok bool) {

	batch = new(clientpb.Batch)

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
		cmd := elem.Value.(*clientpb.Command)
		if serialNo := c.serialNumbers[uint32(cmd.ClientID)]; serialNo >= cmd.SequenceNumber {
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
	return batch, true
}

// Get returns a batch of commands to propose.
func (c *cmdCache) Get(ctx context.Context) (cmd consensus.Command, ok bool) {
	batch, _ := c.getBatch(ctx)
	return c.marshalBatch(batch)
}

// marshalBatch: Internal method used to marshal a batch of commands to a single command string.
func (c *cmdCache) marshalBatch(batch *clientpb.Batch) (cmd consensus.Command, ok bool) {
	// otherwise, we should have at least one command
	b, err := c.marshaler.Marshal(batch)
	if err != nil {
		c.mods.Logger().Errorf("Failed to marshal batch: %v", err)
		return "", false
	}
	cmd = consensus.Command(b)
	return cmd, true
}

// unmarshalCommand: Internal method used to unmarshal a string of command to the underlying batch.
func (c *cmdCache) unmarshalCommand(cmd consensus.Command) (batch *clientpb.Batch, ok bool) {
	// otherwise, we should have at least one command
	batch = new(clientpb.Batch)
	err := c.unmarshaler.Unmarshal([]byte(cmd), batch)
	if err != nil {
		c.mods.Logger().Errorf("Failed to unmarshal batch: %v", err)
		return batch, false
	}
	return batch, true
}

// Accept returns true if the replica can accept the batch.
func (c *cmdCache) Accept(cmd consensus.Command) bool {
	batch, ok := c.unmarshalCommand(cmd)
	if !ok {
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
	batch, ok := c.unmarshalCommand(cmd)
	if !ok {
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

func (c *cmdCache) GetHighestCheckPointedView() consensus.View {
	return consensus.GetGenesis().View()
}

var _ consensus.Acceptor = (*cmdCache)(nil)
var _ consensus.CommandQueue = (*cmdCache)(nil)
