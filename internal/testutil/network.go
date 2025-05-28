package testutil

import (
	"fmt"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
)

type MockReplicaConfig struct {
	EventLoopBufferSize uint
}

type MockReplicaNode struct {
	ID hotstuff.ID

	// Reusable modules
	EventLoop  *eventloop.EventLoop
	Logger     logging.Logger
	Sender     *MockSender
	BlockChain *blockchain.BlockChain

	// private
	network *MockNetwork
}

type MockNetwork struct {
	t        *testing.T
	replicas map[hotstuff.ID]*MockReplicaNode
}

// MockReplicaInitFunc is called upon initializing an individual mock replica, allowing the
// user of the MockNetwork to receive the event loop and sender modules.
type MockReplicaInitFunc func(
	id hotstuff.ID,
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	sender modules.Sender,
	blockChain *blockchain.BlockChain,
)

func NewMockNetwork(t *testing.T, replicaCount uint, config MockReplicaConfig) *MockNetwork {
	if replicaCount <= 1 {
		t.Fatal("need at least 2 replicas")
	}
	n := &MockNetwork{
		t:        t,
		replicas: map[hotstuff.ID]*MockReplicaNode{},
	}
	for i := range replicaCount {
		id := hotstuff.ID(i + 1)
		sender := NewMockSender(id, n)
		logger := logging.New(fmt.Sprintf("test%d", id))
		eventLoop := eventloop.New(logger, config.EventLoopBufferSize)
		replica := &MockReplicaNode{
			ID:         id,
			network:    n,
			EventLoop:  eventLoop,
			Logger:     logger,
			Sender:     sender,
			BlockChain: blockchain.New(eventLoop, logger, sender),
		}
		n.replicas[id] = replica
	}

	ids := n.ReplicaIDs()
	for _, replica := range n.replicas {
		replica.Sender.replicasAvailable = ids
	}
	return n
}

func (n *MockNetwork) ReplicaIDs() []hotstuff.ID {
	keys := make([]hotstuff.ID, 0, len(n.replicas))
	for k := range n.replicas {
		keys = append(keys, k)
	}
	return keys
}

func (n *MockNetwork) Replica(id hotstuff.ID) *MockReplicaNode {
	r, ok := n.replicas[id]
	if !ok {
		n.t.Fatalf("replica node %d not found", id)
	}
	return r
}

func (n *MockNetwork) unicast(id hotstuff.ID, availableToSend []hotstuff.ID, message any) {
	if !idExists(availableToSend, id) {
		return
	}
	n.Replica(id).EventLoop.AddEvent(message)
}

func (n *MockNetwork) broadcast(broadcasterId hotstuff.ID, availableToSend []hotstuff.ID, msg any) {
	for id := range n.replicas {
		if id == broadcasterId {
			continue
		}
		n.unicast(id, availableToSend, msg)
	}
}

func idExists(ids []hotstuff.ID, id hotstuff.ID) bool {
	for _, other := range ids {
		if other == id {
			return true
		}
	}
	return false
}
