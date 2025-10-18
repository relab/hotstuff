package twins

import (
	"context"
	"fmt"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

type emulatedSender struct {
	node      *node
	network   *Network
	subConfig []hotstuff.ID
}

// newSender returns a new emulated sender based on the node and network.
func newSender(n *Network, node *node) *emulatedSender {
	return &emulatedSender{
		network: n,
		node:    node,
	}
}

// broadcastMessage invokes sendMessage for all replicas in the configuration.
func (s *emulatedSender) broadcastMessage(message any) {
	for id := range s.network.replicas {
		if id == s.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		} else if len(s.subConfig) == 0 || slices.Contains(s.subConfig, id) {
			s.sendMessage(id, message)
		}
	}
}

// sendMessage adds a message to the queue in the emulated network. The message is sent
// to the replica in the next tick.
// NOTE: shouldDrop is called to check if the message should be dropped before sending.
func (s *emulatedSender) sendMessage(id hotstuff.ID, message any) {
	nodes, ok := s.network.replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to unknown replica %d", id))
	}
	for _, node := range nodes {
		if s.shouldDrop(node.id, message) {
			s.network.logger.Infof("node %v -> node %v: DROP %T(%v)", s.node.id, node.id, message, message)
			continue
		}
		s.network.logger.Infof("node %v -> node %v: SEND %T(%v)", s.node.id, node.id, message, message)
		s.network.pendingMessages = append(
			s.network.pendingMessages,
			pendingMessage{
				sender:   s.node.id,
				receiver: node.id,
				message:  message,
			},
		)
	}
}

// shouldDrop checks if a message to the node identified by id should be dropped.
func (s *emulatedSender) shouldDrop(id NodeID, message any) bool {
	// retrieve the drop config for this node.
	return s.network.shouldDrop(s.node.id, id, message)
}

// Sub returns a subconfiguration containing the replicas specified in the ids slice.
func (s *emulatedSender) Sub(ids []hotstuff.ID) (sub core.Sender, err error) {
	return &emulatedSender{
		node:      s.node,
		network:   s.network,
		subConfig: ids,
	}, nil
}

// Propose sends the block to all replicas in the configuration.
func (s *emulatedSender) Propose(proposal *hotstuff.ProposeMsg) {
	// very important to dereference it!
	s.broadcastMessage(*proposal)
}

// Timeout sends the timeout message to all replicas.
func (s *emulatedSender) Timeout(msg hotstuff.TimeoutMsg) {
	s.broadcastMessage(msg)
}

// Vote sends the partial cert to the replica.
func (s *emulatedSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	if _, ok := s.network.replicas[id]; !ok {
		return fmt.Errorf("replica with id %d not found", id)
	}
	s.sendMessage(id, hotstuff.VoteMsg{
		ID:          s.node.id.ReplicaID,
		PartialCert: cert,
	})
	return nil
}

// NewView sends the new view message to the replica.
func (s *emulatedSender) NewView(id hotstuff.ID, si hotstuff.SyncInfo) error {
	if _, ok := s.network.replicas[id]; !ok {
		return fmt.Errorf("replica with id %d not found", id)
	}
	s.sendMessage(id, hotstuff.NewViewMsg{
		ID:          s.node.id.ReplicaID,
		SyncInfo:    si,
		FromNetwork: true,
	})
	return nil
}

// RequestBlock requests a block from all the replicas in the network.
func (s *emulatedSender) RequestBlock(_ context.Context, hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
	for _, replica := range s.network.replicas {
		for _, node := range replica {
			if s.shouldDrop(node.id, hash) {
				continue
			}
			block, ok = node.blockchain.LocalGet(hash)
			if ok {
				return block, true
			}
		}
	}
	return nil, false
}

var _ core.Sender = (*emulatedSender)(nil)
