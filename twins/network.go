package twins

import (
	"context"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type Node struct {
	NodeID  uint32
	Modules *consensus.Modules
}

type Network struct {
	Nodes map[uint32]*Node
	// Maps a replica ID to a replica and its twins.
	Replicas map[hotstuff.ID][]*Node
	// For each view (starting at 1), contains the list of partitions for that view.
	ViewPartitions [][]nodeSet
}

// ShouldDrop decides if the sender should drop the message, based on the current view of the sender and the
// partitions configured for that view.
func (n *Network) ShouldDrop(sender, receiver uint32) bool {
	node, ok := n.Nodes[sender]
	if !ok {
		panic(fmt.Errorf("node matching sender id %d was not found", sender))
	}

	// Index into viewPartitions.
	i := int(node.Modules.Synchronizer().View() - 1)

	// will default to dropping all messages from views that don't have any specified partitions.
	if i >= len(n.ViewPartitions) {
		return true
	}

	partitions := n.ViewPartitions[i]
	for _, partition := range partitions {
		if partition.Contains(sender) && partition.Contains(receiver) {
			return false
		}
	}

	return true
}

type configuration struct {
	node    *Node
	network *Network
}

func (c *configuration) broadcastMessage(message interface{}) {
	for id := range c.network.Replicas {
		c.sendMessage(id, message)
	}
}

func (c *configuration) sendMessage(id hotstuff.ID, message interface{}) {
	nodes, ok := c.network.Replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to replica %d, but this replica does not exist", id))
	}
	for _, node := range nodes {
		if c.shouldDrop(node.NodeID) {
			continue
		}
		node.Modules.EventLoop().AddEvent(message)
	}
}

// shouldDrop checks if a message to the node identified by id should be dropped.
func (c *configuration) shouldDrop(id uint32) bool {
	// retrieve the drop config for this node.
	return c.network.ShouldDrop(c.node.NodeID, id)
}

// Replicas returns all of the replicas in the configuration.
func (c *configuration) Replicas() map[hotstuff.ID]consensus.Replica {
	m := make(map[hotstuff.ID]consensus.Replica)
	for id := range c.network.Replicas {
		m[id] = &replica{
			config: c,
			id:     id,
		}
	}
	return m
}

// Replica returns a replica if present in the configuration.
func (c *configuration) Replica(id hotstuff.ID) (r consensus.Replica, ok bool) {
	if _, ok = c.network.Replicas[id]; ok {
		return &replica{
			config: c,
			id:     id,
		}, true
	}
	return nil, false
}

// Len returns the number of replicas in the configuration.
func (c *configuration) Len() int {
	return len(c.network.Replicas)
}

// QuorumSize returns the size of a quorum.
func (c *configuration) QuorumSize() int {
	return hotstuff.QuorumSize(c.Len())
}

// Propose sends the block to all replicas in the configuration.
func (c *configuration) Propose(proposal consensus.ProposeMsg) {
	c.broadcastMessage(proposal)
}

// Timeout sends the timeout message to all replicas.
func (c *configuration) Timeout(msg consensus.TimeoutMsg) {
	c.broadcastMessage(msg)
}

// Fetch requests a block from all the replicas in the configuration.
func (c *configuration) Fetch(_ context.Context, hash consensus.Hash) (block *consensus.Block, ok bool) {
	for _, replica := range c.network.Replicas {
		for _, node := range replica {
			if c.shouldDrop(node.NodeID) {
				continue
			}
			block, ok = node.Modules.BlockChain().LocalGet(hash)
			if ok {
				return block, true
			}
		}
	}
	return nil, false
}

type replica struct {
	// pointer to the node that wants to contact this replica.
	config *configuration
	// id of the replica.
	id hotstuff.ID
}

// ID returns the replica's id.
func (r *replica) ID() hotstuff.ID {
	return r.config.node.Modules.ID()
}

// PublicKey returns the replica's public key.
func (r *replica) PublicKey() consensus.PublicKey {
	return r.config.node.Modules.PrivateKey().Public()
}

// Vote sends the partial certificate to the other replica.
func (r *replica) Vote(cert consensus.PartialCert) {
	r.config.sendMessage(r.id, consensus.VoteMsg{
		ID:          r.config.node.Modules.ID(),
		PartialCert: cert,
	})
}

// NewView sends the quorum certificate to the other replica.
func (r *replica) NewView(si consensus.SyncInfo) {
	r.config.sendMessage(r.id, consensus.NewViewMsg{
		ID:       r.config.node.Modules.ID(),
		SyncInfo: si,
	})
}

type nodeSet map[uint32]struct{}

func (s nodeSet) Add(v uint32) {
	s[v] = struct{}{}
}

func (s nodeSet) Contains(v uint32) bool {
	_, ok := s[v]
	return ok
}
