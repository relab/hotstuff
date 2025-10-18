package twins

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/crypto/keygen"
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

type pendingMessage struct {
	message  any
	sender   NodeID
	receiver NodeID
	view     hotstuff.View
}

func (pm pendingMessage) String() string {
	if pm.message == nil {
		return fmt.Sprintf("%v→%v", pm.sender, pm.receiver)
	}
	return fmt.Sprintf("%v→%v: %v", pm.sender, pm.receiver, pm.message)
}

// Network is a simulated network that supports twins.
type Network struct {
	nodes map[NodeID]*node
	// Maps a replica ID to a replica and its twins.
	replicas map[hotstuff.ID][]*node
	// For each view (starting at 1), contains the list of partitions for that view.
	views []View

	// Global view, to enforce no out of view messages.
	globalView hotstuff.View

	// the message types to drop
	dropTypes map[reflect.Type]struct{}

	pendingMessages []pendingMessage

	logger logging.Logger
	// the destination of the logger
	log strings.Builder
	err error
}

// NewSimpleNetwork creates a simple network.
func NewSimpleNetwork(numNodes int) *Network {
	allNodesSet := make(NodeSet)
	for i := 1; i <= numNodes; i++ {
		allNodesSet.Add(NodeID{ReplicaID: hotstuff.ID(i), NetworkID: uint32(i)})
	}
	network := &Network{
		nodes:      make(map[NodeID]*node),
		replicas:   make(map[hotstuff.ID][]*node),
		views:      []View{{Leader: 1, Partitions: []NodeSet{allNodesSet}}},
		globalView: 1,
		dropTypes:  make(map[reflect.Type]struct{}),
	}
	network.logger = logging.NewWithDest(&network.log, "network")
	return network
}

// NewPartitionedNetwork creates a new Network with the specified partitions.
// partitions specifies the network partitions for each view.
func NewPartitionedNetwork(views []View, dropTypes ...any) *Network {
	n := &Network{
		nodes:      make(map[NodeID]*node),
		replicas:   make(map[hotstuff.ID][]*node),
		views:      views,
		globalView: 1,
		dropTypes:  make(map[reflect.Type]struct{}),
	}
	n.logger = logging.NewWithDest(&n.log, "network")
	for _, t := range dropTypes {
		n.dropTypes[reflect.TypeOf(t)] = struct{}{}
	}
	return n
}

func (n *Network) createTwinsNodes(nodes []NodeID, consensusName string) error {
	lastReplicaID := hotstuff.ID(1)
	lastPrivKey, err := keygen.GenerateECDSAPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key %w", err)
	}
	for _, nodeID := range nodes {
		if nodeID.ReplicaID != lastReplicaID {
			// not a twin, generate new key
			lastPrivKey, err = keygen.GenerateECDSAPrivateKey()
			if err != nil {
				return fmt.Errorf("failed to generate private key %w", err)
			}
		}

		node, err := newNode(n, nodeID, consensusName, lastPrivKey)
		if err != nil {
			return fmt.Errorf("failed to create node %v: %w", nodeID, err)
		}
		n.nodes[nodeID] = node
		n.replicas[nodeID.ReplicaID] = append(n.replicas[nodeID.ReplicaID], node)
	}
	// need to configure the replica info after all of them were set up
	for _, node := range n.nodes {
		config := node.config
		for _, otherNode := range n.nodes {
			node.sender.subConfig = append(node.sender.subConfig, otherNode.id.ReplicaID)
			config.AddReplica(&hotstuff.ReplicaInfo{
				ID:     otherNode.config.ID(),
				PubKey: otherNode.config.PrivateKey().Public(),
			})
		}
	}
	return nil
}

func (n *Network) run(ticks int) {
	// kick off the initial proposal(s)
	for _, node := range n.nodes {
		if node.leaderRotation.GetLeader(1) == node.id.ReplicaID {
			s := node.viewStates
			proposal, err := node.proposer.CreateProposal(s.SyncInfo())
			if err != nil {
				panic(err) // should not fail to create propose, unless command cache has a bug.
			}
			if err := node.proposer.Propose(&proposal); err != nil {
				n.logger.Info(err)
			}
		}
	}

	for tick := 0; tick < ticks; tick++ {
		n.logger.Debugf("new tick: %d", tick)
		n.tick()
		if n.err != nil {
			break
		}
	}
}

// tick adds pending messages to each node's event loop and subsequently performs one tick for each node,
// processing each pending message.
func (n *Network) tick() {
	nextMsgs := make([]pendingMessage, 0, len(n.pendingMessages))
	for _, msg := range n.pendingMessages {
		if msg.view > n.globalView {
			nextMsgs = append(nextMsgs, msg)
			continue
		}
		n.nodes[msg.receiver].eventLoop.AddEvent(msg.message)
	}

	if len(n.pendingMessages) == len(nextMsgs) {
		// no new messages were delivered, advance global view
		n.globalView++
	}

	n.pendingMessages = nextMsgs

	for _, node := range n.nodes {
		node.eventLoop.AddEvent(tick{})
		// process all events in the node's event queue
		for node.eventLoop.Tick(context.Background()) { //revive:disable-line:empty-block
		}
	}

}

// shouldDrop decides if the sender should drop the message, based on the current view of the sender and the
// partitions configured for that view.
func (n *Network) shouldDrop(sender, receiver NodeID, message any, view hotstuff.View) bool {

	// Index into viewPartitions.
	i := int(view) - 1

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

	_, ok := n.dropTypes[reflect.TypeOf(message)]

	return ok
}

// NodeSet is a set of NodeIDs.
type NodeSet map[NodeID]struct{}

// NewNodeSet creates a new NodeSet containing the specified NodeIDs.
func NewNodeSet(ids ...NodeID) NodeSet {
	s := make(NodeSet)
	for _, id := range ids {
		s.Add(id)
	}
	return s
}

// Add adds a NodeID to the set.
func (s NodeSet) Add(v NodeID) {
	s[v] = struct{}{}
}

// Contains returns true if the set contains the NodeID, false otherwise.
func (s NodeSet) Contains(v NodeID) bool {
	_, ok := s[v]
	return ok
}

// MarshalJSON returns a JSON representation of the node set.
func (s NodeSet) MarshalJSON() ([]byte, error) {
	ids := slices.Collect(maps.Keys(s))
	slices.SortFunc(ids, func(a, b NodeID) int {
		if a.ReplicaID != b.ReplicaID {
			return int(a.ReplicaID) - int(b.ReplicaID)
		}
		return int(a.NetworkID) - int(b.NetworkID)
	})
	return json.Marshal(ids)
}

// UnmarshalJSON restores the node set from JSON.
func (s *NodeSet) UnmarshalJSON(data []byte) error {
	if *s == nil {
		*s = make(NodeSet)
	}
	var nodes []NodeID
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
