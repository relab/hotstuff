package fuzz

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/relab/hotstuff"
)

// View specifies the leader id and the partition scenario for a single view.
type View struct {
	Leader     hotstuff.ID `json:"leader"`
	Partitions []NodeSet   `json:"partitions"`
}

// Scenario specifies the nodes, partitions and leaders for a twins scenario.
type Scenario []View

func (s Scenario) String() string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		sb.WriteString(fmt.Sprintf("leader: %d, partitions: ", s[i].Leader))
		for _, partition := range s[i].Partitions {
			sb.WriteString("[ ")
			for id := range partition {
				sb.WriteString(fmt.Sprint(id))
				sb.WriteString(" ")
			}
			sb.WriteString("] ")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ScenarioResult contains the result and logs from executing a scenario.
type ScenarioResult struct {
	Safe         bool
	Commits      int
	NetworkLog   string
	NodeLogs     map[NodeID]string
	NodeCommits  map[NodeID][]*hotstuff.Block
	Messages     []any
	MessageCount int
}

func assignNodeIDs(numNodes, numTwins uint8) (nodes, twins []NodeID) {
	replicaID := hotstuff.ID(1)
	networkID := uint32(1)
	remainingTwins := numTwins

	// assign IDs to nodes
	for i := uint8(0); i < numNodes; i++ {
		if remainingTwins > 0 {
			twins = append(twins, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			networkID++
			twins = append(twins, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			remainingTwins--
		} else {
			nodes = append(nodes, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
		}
		networkID++
		replicaID++
	}

	return
}

// ExecuteScenario executes a twins scenario.
func ExecuteScenario(scenario Scenario, numNodes, numTwins uint8, numTicks int, consensusName string, replaceMessage ...any) (result ScenarioResult, err error) {
	// Network simulator that blocks proposals, votes, and fetch requests between nodes that are in different partitions.
	// Timeout and NewView messages are permitted.

	network := NewPartitionedNetwork(scenario,
		hotstuff.ProposeMsg{},
		hotstuff.VoteMsg{},
		hotstuff.Hash{},
		hotstuff.NewViewMsg{},
		hotstuff.TimeoutMsg{},
	)

	if len(replaceMessage) == 2 {
		network.OldMessage = replaceMessage[0].(int)
		network.NewMessage = replaceMessage[1]
	}

	nodes, twins := assignNodeIDs(numNodes, numTwins)
	nodes = append(nodes, twins...)

	err = network.createTwinsNodes(nodes, scenario, consensusName)
	if err != nil {
		return ScenarioResult{}, err
	}

	network.run(numTicks)

	nodeLogs := make(map[NodeID]string)
	for _, node := range network.nodes {
		nodeLogs[node.id] = node.log.String()
	}

	// check if the majority of replicas have committed the same blocks
	safe, commits := checkCommits(network)

	return ScenarioResult{
		Safe:         safe,
		Commits:      commits,
		NetworkLog:   network.log.String(),
		NodeLogs:     nodeLogs,
		NodeCommits:  getBlocks(network),
		Messages:     network.Messages,
		MessageCount: network.MessageCounter,
	}, nil
}

func checkCommits(network *Network) (safe bool, commits int) {
	i := 0
	for {
		noCommits := true
		commitCount := make(map[hotstuff.Hash]int)
		for _, replica := range network.replicas {
			if len(replica) != 1 {
				// TODO: should we be skipping replicas with twins?
				continue
			}
			if len(replica[0].executedBlocks) <= i {
				continue
			}
			commitCount[replica[0].executedBlocks[i].Hash()]++
			noCommits = false
		}

		if noCommits {
			break
		}

		// if all correct replicas have executed the same blocks, then there should be only one entry in commitCount
		// the number of replicas that committed the block could be smaller, if some correct replicas happened to
		// be in a different partition at the time when the test ended.
		if len(commitCount) != 1 {
			return false, i
		}

		i++
	}
	return true, i
}

type leaderRotation []View

// GetLeader returns the id of the leader in the given view.
func (lr leaderRotation) GetLeader(view hotstuff.View) hotstuff.ID {
	// we start at view 1
	v := int(view) - 1
	if v >= 0 && v < len(lr) {
		return lr[v].Leader
	}
	// default to 0 (which is an invalid id)
	return 0
}

func getBlocks(network *Network) map[NodeID][]*hotstuff.Block {
	m := make(map[NodeID][]*hotstuff.Block)
	for _, node := range network.nodes {
		m[node.id] = node.executedBlocks
	}
	return m
}

type commandGenerator struct {
	mut     sync.Mutex
	nextCmd uint64
}

func (cg *commandGenerator) next() hotstuff.Command {
	cg.mut.Lock()
	defer cg.mut.Unlock()
	cmd := hotstuff.Command(strconv.FormatUint(cg.nextCmd, 10))
	cg.nextCmd++
	return cmd
}

type commandModule struct {
	commandGenerator *commandGenerator
	node             *node
}

// Accept returns true if the replica should accept the command, false otherwise.
func (commandModule) Accept(_ hotstuff.Command) bool {
	return true
}

// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
// accepted in the future.
func (commandModule) Proposed(_ hotstuff.Command) {}

// Get returns the next command to be proposed.
// It may run until the context is cancelled.
// If no command is available, the 'ok' return value should be false.
func (cm commandModule) Get(_ context.Context) (cmd hotstuff.Command, ok bool) {
	return cm.commandGenerator.next(), true
}

// Exec executes the given command.
func (cm commandModule) Exec(block *hotstuff.Block) {
	cm.node.executedBlocks = append(cm.node.executedBlocks, block)
}

func (commandModule) Fork(block *hotstuff.Block) {}
