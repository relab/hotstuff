package twins

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
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
	modules         *modules.ConsensusCore
	executedBlocks  []*hotstuff.Block
	lastMessageView hotstuff.View
	log             strings.Builder
}

// Network is a simulated network that supports twins.
type Network struct {
	nodes map[uint32]*node
	// Maps a replica ID to a replica and its twins.
	replicas map[hotstuff.ID][]*node
	// For each view (starting at 1), contains the list of partitions for that view.
	views []View

	dropTypes map[reflect.Type]struct{}

	logger logging.Logger

	log strings.Builder
}

// NewSimpleNetwork creates a simple network.
func NewSimpleNetwork() *Network {
	return &Network{
		nodes:     make(map[uint32]*node),
		replicas:  make(map[hotstuff.ID][]*node),
		dropTypes: make(map[reflect.Type]struct{}),
	}
}

// NewPartitionedNetwork creates a new Network with the specified partitions.
// partitions specifies the network partitions for each view.
func NewPartitionedNetwork(rounds []View, dropTypes ...any) *Network {
	n := &Network{
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

// GetNodeBuilder returns a consensus.ConsensusBuilder instance for a node in the network.
func (n *Network) GetNodeBuilder(id NodeID, pk hotstuff.PrivateKey) modules.ConsensusBuilder {
	node := node{
		id: id,
	}
	n.nodes[id.NetworkID] = &node
	n.replicas[id.ReplicaID] = append(n.replicas[id.ReplicaID], &node)
	builder := modules.NewConsensusBuilder(id.ReplicaID, pk)
	// register node as an anonymous module because that allows configuration to obtain it.
	builder.Register(&node)
	return builder
}

func (n *Network) createTwinsNodes(nodes []NodeID, scenario Scenario, consensusName string) error {
	cg := &commandGenerator{}
	for _, nodeID := range nodes {

		var err error
		pk, err := keygen.GenerateECDSAPrivateKey()
		if err != nil {
			return err
		}

		builder := n.GetNodeBuilder(nodeID, pk)
		node := n.nodes[nodeID.NetworkID]

		var consensusModule consensus.Rules
		if !modules.GetModule(consensusName, &consensusModule) {
			return fmt.Errorf("unknown consensus module: '%s'", consensusName)
		}
		builder.Register(
			logging.NewWithDest(&node.log, fmt.Sprintf("r%dn%d", nodeID.ReplicaID, nodeID.NetworkID)),
			blockchain.New(),
			consensus.New(consensusModule),
			consensus.NewVotingMachine(),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(FixedTimeout(0)),
			n.NewConfiguration(),
			leaderRotation(scenario),
			commandModule{commandGenerator: cg, node: node},
		)
		builder.OptionsBuilder().SetShouldVerifyVotesSync()
		node.modules = builder.Build()
	}
	return nil
}

// Run runs the nodes for the specified number of rounds.
func (n *Network) Run(rounds int) {
	// kick off the initial proposal(s)
	for _, node := range n.nodes {
		if node.modules.LeaderRotation().GetLeader(1) == node.id.ReplicaID {
			node.modules.Consensus().Propose(node.modules.Synchronizer().(*synchronizer.Synchronizer).SyncInfo())
		}
	}

	for view := hotstuff.View(0); view <= hotstuff.View(rounds); view++ {
		n.Round(view)
	}
}

// Round performs one round for each node.
func (n *Network) Round(view hotstuff.View) {
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
func (n *Network) shouldDrop(sender, receiver uint32, message any) bool {
	// if no partitions are specified for any view, we will allow all messages
	if n.views == nil {
		return false
	}

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

// NewConfiguration returns a new Configuration module for this network.
func (n *Network) NewConfiguration() modules.Configuration {
	return &configuration{network: n}
}

type configuration struct {
	node    *node
	network *Network
}

// alternative way to get a pointer to the node.
func (c *configuration) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	if c.node == nil {
		mods.GetModuleByType(&c.node)
		c.node.modules = mods
	}
}

func (c *configuration) broadcastMessage(message any) {
	for id := range c.network.replicas {
		if id == c.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		}
		c.sendMessage(id, message)
	}
}

func (c *configuration) sendMessage(id hotstuff.ID, message any) {
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
func (c *configuration) shouldDrop(id NodeID, message any) bool {
	// retrieve the drop config for this node.
	return c.network.shouldDrop(c.node.id.NetworkID, id.NetworkID, message)
}

// Replicas returns all of the replicas in the configuration.
func (c *configuration) Replicas() map[hotstuff.ID]modules.Replica {
	m := make(map[hotstuff.ID]modules.Replica)
	for id := range c.network.replicas {
		m[id] = &replica{
			config: c,
			id:     id,
		}
	}
	return m
}

// Replica returns a replica if present in the configuration.
func (c *configuration) Replica(id hotstuff.ID) (r modules.Replica, ok bool) {
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
func (c *configuration) Propose(proposal hotstuff.ProposeMsg) {
	c.broadcastMessage(proposal)
}

// Timeout sends the timeout message to all replicas.
func (c *configuration) Timeout(msg hotstuff.TimeoutMsg) {
	c.broadcastMessage(msg)
}

// Fetch requests a block from all the replicas in the configuration.
func (c *configuration) Fetch(_ context.Context, hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
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
func (r *replica) PublicKey() hotstuff.PublicKey {
	return r.config.network.replicas[r.id][0].modules.PrivateKey().Public()
}

// Vote sends the partial certificate to the other replica.
func (r *replica) Vote(cert hotstuff.PartialCert) {
	r.config.sendMessage(r.id, hotstuff.VoteMsg{
		ID:          r.config.node.modules.ID(),
		PartialCert: cert,
	})
}

// NewView sends the quorum certificate to the other replica.
func (r *replica) NewView(si hotstuff.SyncInfo) {
	r.config.sendMessage(r.id, hotstuff.NewViewMsg{
		ID:       r.config.node.modules.ID(),
		SyncInfo: si,
	})
}

func (r *replica) Metadata() map[string]string {
	return r.config.network.replicas[r.id][0].modules.Options().ConnectionMetadata()
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

// FixedTimeout returns an ExponentialTimeout with a max exponent of 0.
func FixedTimeout(timeout time.Duration) synchronizer.ViewDuration {
	return fixedDuration{timeout}
}

type fixedDuration struct {
	timeout time.Duration
}

func (d fixedDuration) Duration() time.Duration { return d.timeout }
func (d fixedDuration) ViewStarted()            {}
func (d fixedDuration) ViewSucceeded()          {}
func (d fixedDuration) ViewTimeout()            {}
