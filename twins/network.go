package twins

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// NodeID is an ID that is unique to a node in the network.
// The ReplicaID is the ID that the node uses when taking part in the consensus protocol,
// while the NetworkID is used to distinguish nodes on the network.
type NodeID struct {
	ReplicaID hotstuff.ID
	NetworkID uint32
}

func (id NodeID) String() string {
	return fmt.Sprintf("r%dn%d", id.ReplicaID, id.NetworkID)
}

type node struct {
	id              NodeID
	modules         *consensus.Modules
	executedBlocks  []*consensus.Block
	lastMessageView consensus.View
	log             strings.Builder
}

// Network is a simulated network that supports twins.
type network struct {
	nodes map[uint32]*node
	// Maps a replica ID to a replica and its twins.
	replicas map[hotstuff.ID][]*node
	// For each view (starting at 1), contains the list of partitions for that view.
	views []View

	dropTypes map[reflect.Type]struct{}

	logger logging.Logger

	log strings.Builder
}

// newNetwork creates a new Network with the specified partitions.
// partitions specifies the network partitions for each view.
func newNetwork(rounds []View, dropTypes ...interface{}) *network {
	n := &network{
		nodes:     make(map[uint32]*node),
		replicas:  make(map[hotstuff.ID][]*node),
		views:     rounds,
		dropTypes: make(map[reflect.Type]struct{}),
	}
	n.logger = logging.NewWithDest(&n.log, "network")
	for _, t := range dropTypes {
		n.dropTypes[reflect.TypeOf(t)] = struct{}{}
	}
	return n
}

func (n *network) createNodes(nodes []NodeID, scenario Scenario, consensusName string) error {
	cg := &commandGenerator{}
	keys := make(map[hotstuff.ID]consensus.PrivateKey)
	for _, nodeID := range nodes {
		pk, ok := keys[nodeID.ReplicaID]
		if !ok {
			var err error
			pk, err = keygen.GenerateECDSAPrivateKey()
			if err != nil {
				return err
			}
			keys[nodeID.ReplicaID] = pk
		}
		node := node{
			id: nodeID,
		}
		builder := consensus.NewBuilder(nodeID.ReplicaID, pk)
		var consensusModule consensus.Rules
		if !modules.GetModule(consensusName, &consensusModule) {
			return fmt.Errorf("unknown consensus module: '%s'", consensusName)
		}
		builder.Register(
			logging.NewWithDest(&node.log, fmt.Sprintf("r%dn%d", nodeID.ReplicaID, nodeID.NetworkID)),
			blockchain.New(),
			consensus.New(consensusModule),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(testutil.FixedTimeout(0)),
			&configuration{
				node:    &node,
				network: n,
			},
			leaderRotation(scenario),
			commandModule{commandGenerator: cg, node: &node},
		)
		builder.OptionsBuilder().SetShouldVerifyVotesSync()
		node.modules = builder.Build()
		n.nodes[nodeID.NetworkID] = &node
		n.replicas[nodeID.ReplicaID] = append(n.replicas[nodeID.ReplicaID], &node)
	}
	return nil
}

func (n *network) run(rounds int) {
	// kick off the initial proposal(s)
	for _, node := range n.nodes {
		if node.modules.LeaderRotation().GetLeader(1) == node.id.ReplicaID {
			node.modules.Consensus().Propose(node.modules.Synchronizer().(*synchronizer.Synchronizer).SyncInfo())
		}
	}

	for view := consensus.View(0); view <= consensus.View(rounds); view++ {
		n.round(view)
	}
}

// round performs one round for each node
func (n *network) round(view consensus.View) {
	n.logger.Infof("Starting round %d", view)

	for _, node := range n.nodes {
		// run each event loop as long as it has events
		for node.modules.EventLoop().Tick() {
		}
	}

	// give the next leader the opportunity to process votes and propose a new block
	for _, node := range n.nodes {
		if node.modules.LeaderRotation().GetLeader(view+1) == node.modules.ID() {
			for node.modules.EventLoop().Tick() {
			}
		}
	}

	for _, node := range n.nodes {
		// if the node did not send any messages this round, it should timeout
		if node.lastMessageView < view {
			// FIXME: should we add the OnLocalTimeout method to the Synchronizer interface?
			node.modules.Synchronizer().(*synchronizer.Synchronizer).OnLocalTimeout()
		}
	}
}

// shouldDrop decides if the sender should drop the message, based on the current view of the sender and the
// partitions configured for that view.
func (n *network) shouldDrop(sender, receiver uint32, message interface{}) bool {
	node, ok := n.nodes[sender]
	if !ok {
		panic(fmt.Errorf("node matching sender id %d was not found", sender))
	}

	// Index into viewPartitions.
	i := int(node.modules.Synchronizer().View() - 1)

	// will default to dropping all messages from views that don't have any specified partitions.
	if i >= len(n.views) {
		return true
	}

	partitions := n.views[i].Partitions
	for _, partition := range partitions {
		if partition.Contains(sender) && partition.Contains(receiver) {
			return false
		}
	}

	_, ok = n.dropTypes[reflect.TypeOf(message)]

	return ok
}

type configuration struct {
	node    *node
	network *network
}

func (c *configuration) broadcastMessage(message interface{}) {
	for id := range c.network.replicas {
		if id == c.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		}
		c.sendMessage(id, message)
	}
}

func (c *configuration) sendMessage(id hotstuff.ID, message interface{}) {
	nodes, ok := c.network.replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to replica %d, but this replica does not exist", id))
	}
	for _, node := range nodes {
		if c.shouldDrop(node.id, message) {
			continue
		}
		c.network.logger.Infof("node %v -> node %v: %T(%v)", c.node.id, node.id, message, message)
		node.modules.EventLoop().AddEvent(message)
	}
	c.node.lastMessageView = c.node.modules.Synchronizer().View()
}

// shouldDrop checks if a message to the node identified by id should be dropped.
func (c *configuration) shouldDrop(id NodeID, message interface{}) bool {
	// retrieve the drop config for this node.
	return c.network.shouldDrop(c.node.id.NetworkID, id.NetworkID, message)
}

// Replicas returns all of the replicas in the configuration.
func (c *configuration) Replicas() map[hotstuff.ID]consensus.Replica {
	m := make(map[hotstuff.ID]consensus.Replica)
	for id := range c.network.replicas {
		m[id] = &replica{
			config: c,
			id:     id,
		}
	}
	return m
}

// Replica returns a replica if present in the configuration.
func (c *configuration) Replica(id hotstuff.ID) (r consensus.Replica, ok bool) {
	if _, ok = c.network.replicas[id]; ok {
		return &replica{
			config: c,
			id:     id,
		}, true
	}
	return nil, false
}

// Len returns the number of replicas in the configuration.
func (c *configuration) Len() int {
	return len(c.network.replicas)
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
	for _, replica := range c.network.replicas {
		for _, node := range replica {
			if c.shouldDrop(node.id, hash) {
				continue
			}
			block, ok = node.modules.BlockChain().LocalGet(hash)
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
	return r.config.network.replicas[r.id][0].id.ReplicaID
}

// PublicKey returns the replica's public key.
func (r *replica) PublicKey() consensus.PublicKey {
	return r.config.network.replicas[r.id][0].modules.PrivateKey().Public()
}

// Vote sends the partial certificate to the other replica.
func (r *replica) Vote(cert consensus.PartialCert) {
	r.config.sendMessage(r.id, consensus.VoteMsg{
		ID:          r.config.node.modules.ID(),
		PartialCert: cert,
	})
}

// NewView sends the quorum certificate to the other replica.
func (r *replica) NewView(si consensus.SyncInfo) {
	r.config.sendMessage(r.id, consensus.NewViewMsg{
		ID:       r.config.node.modules.ID(),
		SyncInfo: si,
	})
}

// NodeSet is a set of network ids.
type NodeSet map[uint32]struct{}

// Add adds a NodeID to the set.
func (s NodeSet) Add(v uint32) {
	s[v] = struct{}{}
}

// Contains returns true if the set contains the NodeID, false otherwise.
func (s NodeSet) Contains(v uint32) bool {
	_, ok := s[v]
	return ok
}

// MarshalJSON returns a JSON representation of the node set.
func (s NodeSet) MarshalJSON() ([]byte, error) {
	nodes := make([]uint32, 0, len(s))
	for node := range s {
		i := sort.Search(len(nodes), func(i int) bool { return node < nodes[i] })
		nodes = append(nodes, 0)
		copy(nodes[i+1:], nodes[i:])
		nodes[i] = node
	}
	return json.Marshal(nodes)
}

// UnmarshalJSON restores the node set from JSON.
func (s *NodeSet) UnmarshalJSON(data []byte) error {
	if *s == nil {
		*s = make(NodeSet)
	}
	var nodes []uint32
	err := json.Unmarshal(data, &nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		s.Add(node)
	}
	return nil
}
