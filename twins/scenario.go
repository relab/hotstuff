package twins

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
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
	Safe        bool
	Commits     int
	NetworkLog  string
	NodeLogs    map[NodeID]string
	NodeCommits map[NodeID][]*hotstuff.Block
}

// ExecuteScenario executes a twins scenario.
func ExecuteScenario(scenario Scenario, numNodes, numTwins uint8, numTicks int, consensusName string) (result ScenarioResult, err error) {
	// Network simulator that blocks proposals, votes, and fetch requests between nodes that are in different partitions.
	// Timeout and NewView messages are permitted.
	network := NewPartitionedNetwork(scenario,
		hotstuff.ProposeMsg{},
		hotstuff.VoteMsg{},
		hotstuff.Hash{},
		hotstuff.NewViewMsg{},
		hotstuff.TimeoutMsg{},
	)

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
		Safe:        safe,
		Commits:     commits,
		NetworkLog:  network.log.String(),
		NodeLogs:    nodeLogs,
		NodeCommits: getBlocks(network),
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

var _ modules.LeaderRotation = (leaderRotation)(nil)

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

func (lr leaderRotation) ViewDuration() modules.ViewDuration {
	return FixedTimeout(0) // TODO(AlanRostem): add correct value
}

func getBlocks(network *Network) map[NodeID][]*hotstuff.Block {
	m := make(map[NodeID][]*hotstuff.Block)
	for _, node := range network.nodes {
		m[node.id] = node.executedBlocks
	}
	return m
}

type commandGenerator struct {
	nextCmd uint64
}

func (cg *commandGenerator) next() *clientpb.Command {
	data := []byte(strconv.FormatUint(cg.nextCmd, 10))
	cg.nextCmd++
	return &clientpb.Command{
		ClientID:       1,
		SequenceNumber: cg.nextCmd,
		Data:           data,
	}
}
