package fuzz

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
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
	blockChain     modules.BlockChain
	consensus      modules.Consensus
	eventLoop      *eventloop.EventLoop
	leaderRotation modules.LeaderRotation
	synchronizer   modules.Synchronizer
	opts           *modules.Options

	id             NodeID
	executedBlocks []*hotstuff.Block
	effectiveView  hotstuff.View
	log            strings.Builder
}

func (n *node) InitModule(mods *modules.Core) {
	mods.Get(
		&n.blockChain,
		&n.consensus,
		&n.eventLoop,
		&n.leaderRotation,
		&n.synchronizer,
		&n.opts,
	)
}

type pendingMessage struct {
	message  any
	receiver uint32
}

// Network is a simulated network that supports twins.
type Network struct {
	nodes map[uint32]*node
	// Maps a replica ID to a replica and its twins.
	replicas map[hotstuff.ID][]*node
	// For each view (starting at 1), contains the list of partitions for that view.
	views []View

	OldMessage int
	NewMessage any

	Messages []any

	MessageCounter int

	// the message types to drop
	dropTypes map[reflect.Type]struct{}

	pendingMessages []pendingMessage

	logger logging.Logger
	// the destination of the logger
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
func NewPartitionedNetwork(views []View, dropTypes ...any) *Network {
	n := &Network{
		nodes:     make(map[uint32]*node),
		replicas:  make(map[hotstuff.ID][]*node),
		views:     views,
		dropTypes: make(map[reflect.Type]struct{}),
	}
	n.logger = logging.NewWithDest(&n.log, "network")
	for _, t := range dropTypes {
		n.dropTypes[reflect.TypeOf(t)] = struct{}{}
	}
	return n
}

// GetNodeBuilder returns a consensus.Builder instance for a node in the network.
func (n *Network) GetNodeBuilder(id NodeID, pk hotstuff.PrivateKey) modules.Builder {
	node := node{
		id: id,
	}
	n.nodes[id.NetworkID] = &node
	n.replicas[id.ReplicaID] = append(n.replicas[id.ReplicaID], &node)
	builder := modules.NewBuilder(id.ReplicaID, pk)
	// register node as an anonymous module because that allows configuration to obtain it.
	builder.Add(&node)
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

		consensusModule, ok := modules.GetModule[consensus.Rules](consensusName)
		if !ok {
			return fmt.Errorf("unknown consensus module: '%s'", consensusName)
		}
		builder.Add(
			eventloop.New(100),
			blockchain.New(),
			consensus.New(consensusModule),
			consensus.NewVotingMachine(),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(FixedTimeout(0)),
			logging.NewWithDest(&node.log, fmt.Sprintf("r%dn%d", nodeID.ReplicaID, nodeID.NetworkID)),
			// twins-specific:
			&configuration{network: n, node: node},
			&timeoutManager{network: n, node: node, timeout: 5},
			leaderRotation(n.views),
			&commandModule{commandGenerator: cg, node: node},
		)
		builder.Options().SetShouldVerifyVotesSync()
		builder.Build()
	}
	return nil
}

func (n *Network) run(ticks int) {
	// kick off the initial proposal(s)
	for _, node := range n.nodes {
		if node.leaderRotation.GetLeader(1) == node.id.ReplicaID {
			node.consensus.Propose(node.synchronizer.(*synchronizer.Synchronizer).SyncInfo())
		}
	}

	for tick := 0; tick < ticks; tick++ {
		n.tick()
	}
}

// tick performs one tick for each node
func (n *Network) tick() {
	for _, msg := range n.pendingMessages {
		n.nodes[msg.receiver].eventLoop.AddEvent(msg.message)
	}
	n.pendingMessages = nil

	for _, node := range n.nodes {
		node.eventLoop.AddEvent(tick{})
		// run each event loop as long as it has events
		for node.eventLoop.Tick() {
		}
	}
}

// shouldDrop decides if the sender should drop the message, based on the current view of the sender and the
// partitions configured for that view.
func (n *Network) shouldDrop(sender, receiver uint32, message any) bool {

	node, ok := n.nodes[sender]
	if !ok {
		panic(fmt.Errorf("node matching sender id %d was not found", sender))
	}

	// Index into viewPartitions.
	i := -1
	if node.effectiveView > node.synchronizer.View() {
		i += int(node.effectiveView)
	} else {
		i += int(node.synchronizer.View())
	}

	if i < 0 {
		return false
	}

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

func (n *Network) shouldSwap(message any) bool {
	n.logger.Infof("is %d equal to %d?", n.OldMessage, n.MessageCounter)
	return n.OldMessage == n.MessageCounter
}

// NewConfiguration returns a new Configuration module for this network.
func (n *Network) NewConfiguration() modules.Configuration {
	return &configuration{network: n}
}

type configuration struct {
	node      *node
	network   *Network
	subConfig hotstuff.IDSet
}

// alternative way to get a pointer to the node.
func (c *configuration) InitModule(mods *modules.Core) {
	if c.node == nil {
		mods.TryGet(&c.node)
	}
}

func (c *configuration) broadcastMessage(message any) {
	c.network.logger.Infof("broadcasting message")
	for id := range c.network.replicas {
		if id == c.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		} else if c.subConfig == nil || c.subConfig.Contains(id) {
			c.sendMessage(id, message)
		}
	}
}

func (c *configuration) sendMessage(id hotstuff.ID, message any) {

	nodes, ok := c.network.replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to replica %d, but this replica does not exist", id))
	}

	for _, node := range nodes {

		if c.shouldDrop(node.id, message) {
			c.network.logger.Infof("node %v -> node %v: DROP %T(%v)", c.node.id, node.id, message, message)
			continue
		}

		c.network.Messages = append(c.network.Messages, message)
		c.network.MessageCounter += 1

		if c.network.shouldSwap(message) {

			/*if (c.network.NewMessage.(hotstuff.ProposeMsg).Block.Parent() == hotstuff.Hash{}) {
				c.network.NewMessage.(hotstuff.ProposeMsg).Block.SetParent(message.(hotstuff.ProposeMsg).Block.Parent())
			}*/
			c.network.logger.Infof("swapping message with fuzz message")
			message = c.network.NewMessage
		}

		c.network.logger.Infof("node %v -> node %v: SEND %T(%v)", c.node.id, node.id, message, message)
		c.network.pendingMessages = append(
			c.network.pendingMessages,
			pendingMessage{
				receiver: uint32(node.id.NetworkID),
				message:  message,
			},
		)
	}
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

// SubConfig returns a subconfiguration containing the replicas specified in the ids slice.
func (c *configuration) SubConfig(ids []hotstuff.ID) (sub modules.Configuration, err error) {
	subConfig := hotstuff.NewIDSet()
	for _, id := range ids {
		subConfig.Add(id)
	}
	return &configuration{
		node:      c.node,
		network:   c.network,
		subConfig: subConfig,
	}, nil
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
			block, ok = node.blockChain.LocalGet(hash)
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
	return r.config.network.replicas[r.id][0].opts.PrivateKey().Public()
}

// Vote sends the partial certificate to the other replica.
func (r *replica) Vote(cert hotstuff.PartialCert) {
	r.config.sendMessage(r.id, hotstuff.VoteMsg{
		ID:          r.config.node.opts.ID(),
		PartialCert: cert,
	})
}

// NewView sends the quorum certificate to the other replica.
func (r *replica) NewView(si hotstuff.SyncInfo) {
	r.config.sendMessage(r.id, hotstuff.NewViewMsg{
		ID:       r.config.node.opts.ID(),
		SyncInfo: si,
	})
}

func (r *replica) Metadata() map[string]string {
	return r.config.network.replicas[r.id][0].opts.ConnectionMetadata()
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
	ids := maps.Keys(s)
	slices.Sort(ids)
	return json.Marshal(ids)
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

type tick struct{}

type timeoutManager struct {
	synchronizer modules.Synchronizer
	eventLoop    *eventloop.EventLoop

	node      *node
	network   *Network
	countdown int
	timeout   int
}

func (tm *timeoutManager) advance() {
	tm.countdown--
	if tm.countdown == 0 {
		view := tm.synchronizer.View()
		tm.eventLoop.AddEvent(synchronizer.TimeoutEvent{View: view})
		tm.countdown = tm.timeout
		if tm.node.effectiveView <= view {
			tm.node.effectiveView = view + 1
			tm.network.logger.Infof("node %v effective view is %d due to timeout", tm.node.id, tm.node.effectiveView)
		}
	}
}

func (tm *timeoutManager) viewChange(event synchronizer.ViewChangeEvent) {
	tm.countdown = tm.timeout
	if event.Timeout {
		tm.network.logger.Infof("node %v entered view %d after timeout", tm.node.id, event.View)
	} else {
		tm.network.logger.Infof("node %v entered view %d after voting", tm.node.id, event.View)
	}
}

// InitModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (tm *timeoutManager) InitModule(mods *modules.Core) {
	mods.Get(
		&tm.synchronizer,
		&tm.eventLoop,
	)

	tm.eventLoop.RegisterObserver(tick{}, func(event any) {
		tm.advance()
	})
	tm.eventLoop.RegisterObserver(synchronizer.ViewChangeEvent{}, func(event any) {
		tm.viewChange(event.(synchronizer.ViewChangeEvent))
	})
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
