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
	"github.com/relab/hotstuff/security/cert"
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
	sender         *emulatedSender
	blockChain     *blockchain.BlockChain
	commandCache   *clientpb.Cache
	voter          *consensus.Voter
	proposer       *consensus.Proposer
	eventLoop      *eventloop.EventLoop
	viewStates     *consensus.ViewStates
	leaderRotation modules.LeaderRotation
	synchronizer   *synchronizer.Synchronizer
	timeoutManager *timeoutManager

	id             NodeID
	executedBlocks []*hotstuff.Block
	effectiveView  hotstuff.View
	log            *strings.Builder
}

func newNode(n *Network, nodeID NodeID, consensusName, cryptoName string) (*node, error) {
	pk, err := keygen.GenerateECDSAPrivateKey()
	if err != nil {
		return nil, err
	}
	log := &strings.Builder{}
	logger := logging.NewWithDest(log, "network")
	node := &node{
		id:           nodeID,
		config:       core.NewRuntimeConfig(nodeID.ReplicaID, pk, core.WithSyncVoteVerification()),
		logger:       logger,
		eventLoop:    eventloop.New(logger, 100),
		commandCache: clientpb.New(),
		log:          log,
	}
	node.sender = &emulatedSender{
		node:      node,
		network:   n,
		subConfig: hotstuff.NewIDSet(),
	}
	depsSecurity, err := wiring.NewSecurity(
		node.eventLoop,
		node.logger,
		node.config,
		node.sender,
		cryptoName,
		cert.WithCache(100),
	)
	if err != nil {
		return nil, err
	}
	node.blockChain = depsSecurity.BlockChain()
	consensusRules, err := wiring.NewConsensusRules(node.logger, node.config, node.blockChain, consensusName)
	if err != nil {
		return nil, err
	}
	committer := committer.New(node.eventLoop, node.logger, node.blockChain, consensusRules)
	node.viewStates, err = consensus.NewViewStates(node.blockChain, depsSecurity.Authority())
	if err != nil {
		return nil, err
	}
	node.leaderRotation = leaderRotation(n.views)
	protocol := consensus.NewHotStuff(
		node.logger,
		node.eventLoop,
		node.config,
		node.blockChain,
		depsSecurity.Authority(),
		node.viewStates,
		node.leaderRotation,
		node.sender,
	)
	node.voter = consensus.NewVoter(
		node.eventLoop,
		node.logger,
		node.config,
		node.leaderRotation,
		consensusRules,
		protocol,
		depsSecurity.Authority(),
		node.commandCache,
		committer,
	)
	node.proposer = consensus.NewProposer(
		node.eventLoop,
		node.logger,
		node.config,
		node.blockChain,
		protocol,
		node.voter,
		node.commandCache,
		committer,
	)
	node.synchronizer = synchronizer.New(
		node.eventLoop,
		node.logger,
		node.config,
		depsSecurity.Authority(),
		node.leaderRotation,
		FixedTimeout(1*time.Millisecond),
		node.proposer,
		node.voter,
		node.viewStates,
		node.sender,
	)
	node.timeoutManager = newTimeoutManager(n, node, node.eventLoop, node.viewStates)
	// necessary to count executed commands.
	node.eventLoop.RegisterHandler(hotstuff.CommitEvent{}, func(event any) {
		commit := event.(hotstuff.CommitEvent)
		node.executedBlocks = append(node.executedBlocks, commit.Block)
	})
	commandGenerator := &commandGenerator{}
	for range n.views {
		cmd := commandGenerator.next()
		node.commandCache.Add(cmd)
	}
	return node, nil
}

type pendingMessage struct {
	message  any
	sender   uint32
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
		node, err := newNode(n, nodeID, consensusName, cryptoName)
		errs = errors.Join(err)
		n.nodes[nodeID.NetworkID] = node
		n.replicas[nodeID.ReplicaID] = append(n.replicas[nodeID.ReplicaID], node)
	}
	// TODO(AlanRostem): set the connection metadata?
	// need to configure the replica info after all of them were set up
	for _, node := range n.nodes {
		config := node.config
		for _, otherNode := range n.nodes {
			node.sender.subConfig.Add(otherNode.id.ReplicaID)
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

func (s *emulatedSender) broadcastMessage(message any) {
	for id := range s.network.replicas {
		if id == s.node.id.ReplicaID {
			// do not send message to self or twin
			continue
		} else if s.subConfig == nil || s.subConfig.Contains(id) {
			s.sendMessage(id, message)
		}
	}
}

func (s *emulatedSender) sendMessage(id hotstuff.ID, message any) {
	nodes, ok := s.network.replicas[id]
	if !ok {
		panic(fmt.Errorf("attempt to send message to replica %d, but this replica does not exist", id))
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
				sender:   uint32(s.node.id.NetworkID),
				receiver: uint32(node.id.NetworkID),
				message:  message,
			},
		)
	}
}

// shouldDrop checks if a message to the node identified by id should be dropped.
func (s *emulatedSender) shouldDrop(id NodeID, message any) bool {
	// retrieve the drop config for this node.
	return s.network.shouldDrop(s.node.id.NetworkID, id.NetworkID, message)
}

// GetSubConfig returns a subconfiguration containing the replicas specified in the ids slice.
func (s *emulatedSender) Sub(ids []hotstuff.ID) (sub modules.Sender, err error) {
	subConfig := hotstuff.NewIDSet()
	for _, id := range ids {
		subConfig.Add(id)
	}
	return &emulatedSender{
		node:      s.node,
		network:   s.network,
		subConfig: subConfig,
	}, nil
}

// Propose sends the block to all replicas in the configuration.
func (s *emulatedSender) Propose(proposal *hotstuff.ProposeMsg) {
	s.broadcastMessage(*proposal) // very important to dereference it!
}

// Timeout sends the timeout message to all replicas.
func (s *emulatedSender) Timeout(msg hotstuff.TimeoutMsg) {
	s.broadcastMessage(msg)
}

func (s *emulatedSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	s.sendMessage(id, hotstuff.VoteMsg{
		ID:          s.node.id.ReplicaID,
		PartialCert: cert,
	})
	return nil
}

func (s *emulatedSender) NewView(id hotstuff.ID, si hotstuff.SyncInfo) error {
	s.sendMessage(id, hotstuff.NewViewMsg{
		ID:       s.node.id.ReplicaID,
		SyncInfo: si,
	})
	return nil
}

// Fetch requests a block from all the replicas in the configuration.
func (s *emulatedSender) RequestBlock(_ context.Context, hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
	for _, replica := range s.network.replicas {
		for _, node := range replica {
			if s.shouldDrop(node.id, hash) {
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
	eventLoop  *eventloop.EventLoop
	viewStates *consensus.ViewStates

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
	network *Network,
	node *node,
	eventLoop *eventloop.EventLoop,
	viewStates *consensus.ViewStates,
) *timeoutManager {
	tm := &timeoutManager{
		node:       node,
		network:    network,
		eventLoop:  eventLoop,
		viewStates: viewStates,
		timeout:    5,
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
