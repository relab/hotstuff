package twins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/keygen"
	"github.com/relab/hotstuff/wiring"
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

// TODO(AlanRostem): initialize fields
type node struct {
	config         *core.RuntimeConfig
	logger         logging.Logger
	blockChain     *blockchain.BlockChain
	commandCache   *clientpb.Cache
	voter          *consensus.Voter
	proposer       *consensus.Proposer
	eventLoop      *eventloop.EventLoop
	viewStates     *consensus.ViewStates
	leaderRotation modules.LeaderRotation
	synchronizer   synchronizer.Synchronizer
	// opts           *core.Options

	id             NodeID
	executedBlocks []*hotstuff.Block
	effectiveView  hotstuff.View
	log            strings.Builder
	cmdGen         *commandGenerator
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

// TODO(AlanRostem): hook something to the execute events
func (n *Network) createTwinsNodes(nodes []NodeID, _ Scenario, consensusName string) (errs error) {
	cryptoName := ecdsa.ModuleName
	for _, nodeID := range nodes {
		var err error
		pk, err := keygen.GenerateECDSAPrivateKey()
		if err != nil {
			errs = errors.Join(err)
			continue
		}
		node := &node{}
		depsCore := wiring.NewCore(nodeID.ReplicaID, "twin", pk)
		sender := &emulatedSender{
			node:    node,
			network: n,
		}
		depsSecurity, err := wiring.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			cryptoName,
		)
		if err != nil {
			errs = errors.Join(err)
			continue
		}
		consensusRules, err := wiring.NewConsensusRules(
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecurity.BlockChain(),
			consensusName,
		)
		if err != nil {
			errs = errors.Join(err)
			continue
		}
		committer := committer.New(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsSecurity.BlockChain(),
			consensusRules,
		)
		viewStates, err := consensus.NewViewStates(
			depsSecurity.BlockChain(),
			depsSecurity.Authority(),
		)
		if err != nil {
			errs = errors.Join(err)
			continue
		}
		leaderRotation := &leaderRotation{}
		protocol := consensus.NewHotStuff(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecurity.BlockChain(),
			depsSecurity.Authority(),
			viewStates,
			leaderRotation,
			sender,
		)
		commandCache := clientpb.New()
		voter := consensus.NewVoter(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			leaderRotation,
			consensusRules,
			protocol,
			depsSecurity.Authority(),
			commandCache,
			committer,
		)
		proposer := consensus.NewProposer(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecurity.BlockChain(),
			protocol,
			voter,
			commandCache,
			committer,
		)
		// TODO(AlanRostem): set this up in a nicer way
		node.config = depsCore.RuntimeCfg()
		node.eventLoop = depsCore.EventLoop()
		node.logger = depsCore.Logger()
		node.commandCache = commandCache
		node.blockChain = depsSecurity.BlockChain()
		node.leaderRotation = leaderRotation
		node.viewStates = viewStates
		node.voter = voter
		node.proposer = proposer
		node.cmdGen = &commandGenerator{}
		n.nodes[nodeID.NetworkID] = node
	}
	// need to configure the replica info after all of them were set up
	// TODO(AlanRostem): is this the correct way?
	// TODO(AlanRostem): set the connection metadata
	for _, node := range n.nodes {
		config := node.config
		for _, otherNode := range n.nodes {
			config.AddReplica(&hotstuff.ReplicaInfo{
				ID:     otherNode.config.ID(),
				PubKey: otherNode.config.PrivateKey().Public(),
			})
		}
	}
	return
}

func (n *Network) run(ticks int) {
	// kick off the initial proposal(s)
	for _, node := range n.nodes {
		// artificially add a command
		cmd := node.cmdGen.next()
		node.commandCache.Add(cmd)
		if node.leaderRotation.GetLeader(1) == node.id.ReplicaID {
			s := node.viewStates
			proposal, err := node.proposer.CreateProposal(s.View(), s.HighQC(), s.SyncInfo())
			if err != nil {
				node.logger.Infof("failed to create proposal: %w", err)
				continue
			}
			node.proposer.Propose(&proposal)
		}
	}

	for tick := 0; tick < ticks; tick++ {
		for _, node := range n.nodes {
			// continue artificially adding commands for each tick
			cmd := node.cmdGen.next()
			node.commandCache.Add(cmd)
		}
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
		// process all events in the node's event queue
		for node.eventLoop.Tick(context.Background()) { //revive:disable-line:empty-block
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
	if node.effectiveView > node.viewStates.View() {
		i += int(node.effectiveView)
	} else {
		i += int(node.viewStates.View())
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

// NewSender returns a new Configuration module for this network.
func (n *Network) NewSender(node *node) modules.Sender {
	return &emulatedSender{
		network: n,
		node:    node,
	}
}

type emulatedSender struct {
	node      *node
	network   *Network
	subConfig hotstuff.IDSet
}

var _ modules.Sender = (*emulatedSender)(nil)

func (c *emulatedSender) broadcastMessage(message any) {
	for id := range c.network.replicas {
		if id == c.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		} else if c.subConfig == nil || c.subConfig.Contains(id) {
			c.sendMessage(id, message)
		}
	}
}

func (c *emulatedSender) sendMessage(id hotstuff.ID, message any) {
	nodes, ok := c.network.replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to replica %d, but this replica does not exist", id))
	}
	for _, node := range nodes {
		if c.shouldDrop(node.id, message) {
			c.network.logger.Infof("node %v -> node %v: DROP %T(%v)", c.node.id, node.id, message, message)
			continue
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
func (c *emulatedSender) shouldDrop(id NodeID, message any) bool {
	// retrieve the drop config for this node.
	return c.network.shouldDrop(c.node.id.NetworkID, id.NetworkID, message)
}

// TODO(AlanRostem): these methods are in core.RunTimeConfig, ensure that they are correct when used here.
/*// Replicas returns all of the replicas in the configuration.
func (c *sender) Replicas() map[hotstuff.ID]*hotstuff.ReplicaInfo {
	m := make(map[hotstuff.ID]*hotstuff.ReplicaInfo)
	for id := range c.network.replicas {
		m[id] = &hotstuff.ReplicaInfo{
			ID: id, // TODO: More fields
		}
	}
	return m
}

// Replica returns a replica if present in the configuration.
func (c *sender) Replica(id hotstuff.ID) (r *hotstuff.ReplicaInfo, ok bool) {
	if _, ok = c.network.replicas[id]; ok {
		return &hotstuff.ReplicaInfo{
			ID: id, // TODO: More fields
		}, true
	}
	return nil, false
}*/

// GetSubConfig returns a subconfiguration containing the replicas specified in the ids slice.
func (c *emulatedSender) Sub(ids []hotstuff.ID) (sub modules.Sender, err error) {
	subConfig := hotstuff.NewIDSet()
	for _, id := range ids {
		subConfig.Add(id)
	}
	return &emulatedSender{
		node:      c.node,
		network:   c.network,
		subConfig: subConfig,
	}, nil
}

// Propose sends the block to all replicas in the configuration.
func (c *emulatedSender) Propose(proposal *hotstuff.ProposeMsg) {
	c.broadcastMessage(proposal)
}

// Timeout sends the timeout message to all replicas.
func (c *emulatedSender) Timeout(msg hotstuff.TimeoutMsg) {
	c.broadcastMessage(msg)
}

func (c *emulatedSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	c.sendMessage(id, hotstuff.VoteMsg{
		ID:          c.node.id.ReplicaID,
		PartialCert: cert,
	})
	return nil
}

func (c *emulatedSender) NewView(id hotstuff.ID, si hotstuff.SyncInfo) error {
	c.sendMessage(id, hotstuff.NewViewMsg{
		ID:       c.node.id.ReplicaID,
		SyncInfo: si,
	})
	return nil
}

// Fetch requests a block from all the replicas in the configuration.
func (c *emulatedSender) RequestBlock(_ context.Context, hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
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

/*type replica struct {
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
}*/

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
	ids := slices.Sorted(maps.Keys(s))
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
	viewStates   *consensus.ViewStates
	synchronizer *synchronizer.Synchronizer
	eventLoop    *eventloop.EventLoop

	node      *node
	network   *Network
	countdown int
	timeout   int
}

func (tm *timeoutManager) advance() {
	tm.countdown--
	if tm.countdown == 0 {
		view := tm.viewStates.View()
		tm.eventLoop.AddEvent(hotstuff.TimeoutEvent{View: view})
		tm.countdown = tm.timeout
		if tm.node.effectiveView <= view {
			tm.node.effectiveView = view + 1
			tm.network.logger.Infof("node %v effective view is %d due to timeout", tm.node.id, tm.node.effectiveView)
		}
	}
}

func (tm *timeoutManager) viewChange(event hotstuff.ViewChangeEvent) {
	tm.countdown = tm.timeout
	if event.Timeout {
		tm.network.logger.Infof("node %v entered view %d after timeout", tm.node.id, event.View)
	} else {
		tm.network.logger.Infof("node %v entered view %d after voting", tm.node.id, event.View)
	}
}

func newTimeoutManager(
	eventLoop *eventloop.EventLoop,
	synchronizer *synchronizer.Synchronizer,
) *timeoutManager {
	tm := &timeoutManager{
		eventLoop:    eventLoop,
		synchronizer: synchronizer,
	}
	tm.eventLoop.RegisterHandler(tick{}, func(_ any) {
		tm.advance()
	}, eventloop.Prioritize())
	tm.eventLoop.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		tm.viewChange(event.(hotstuff.ViewChangeEvent))
	}, eventloop.Prioritize())
	return tm
}

// FixedTimeout returns an ExponentialTimeout with a max exponent of 0.
func FixedTimeout(timeout time.Duration) modules.ViewDuration {
	return fixedDuration{timeout}
}

type fixedDuration struct {
	timeout time.Duration
}

func (d fixedDuration) Duration() time.Duration { return d.timeout }
func (d fixedDuration) ViewStarted()            {}
func (d fixedDuration) ViewSucceeded()          {}
func (d fixedDuration) ViewTimeout()            {}
