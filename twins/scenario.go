package twins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// View specifies the leader id an the partition scenario for a single round of consensus.
type View struct {
	Leader     hotstuff.ID
	Partitions []NodeSet
}

// Scenario specifies the nodes, partitions and leaders for a twins scenario.
type Scenario struct {
	Nodes []NodeID
	Views []View
}

func (s Scenario) String() string {
	var sb strings.Builder
	for i := 0; i < len(s.Views); i++ {
		sb.WriteString(fmt.Sprintf("leader: %d, partitions: ", s.Views[i].Leader))
		for _, partition := range s.Views[i].Partitions {
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

// ExecuteScenario executes a twins scenario.
func ExecuteScenario(scenario Scenario, consensusName string, viewTimeout time.Duration) (safe bool, commits int, err error) {
	// Network simulator that blocks proposals, votes, and fetch requests between nodes that are in different partitions.
	// Timeout and NewView messages are permitted.
	network := newNetwork(scenario.Views, consensus.ProposeMsg{}, consensus.VoteMsg{}, consensus.Hash{})

	ctx, cancel := context.WithCancel(context.Background())

	err = createNodes(scenario, network, consensusName, viewTimeout)
	if err != nil {
		cancel()
		return false, 0, err
	}

	network.startNodes(ctx)
	network.waitUntilHung()
	cancel()
	network.waitUntilDone()

	// check if the majority of replicas have committed the same blocks
	safe, commits = checkCommits(network)

	return safe, commits, nil
}

func createNodes(scenario Scenario, network *network, consensusName string, viewTimeout time.Duration) error {
	cg := &commandGenerator{}
	keys := make(map[hotstuff.ID]consensus.PrivateKey)
	for _, nodeID := range scenario.Nodes {
		pk, ok := keys[nodeID.ReplicaID]
		if !ok {
			var err error
			pk, err = keygen.GenerateECDSAPrivateKey()
			if err != nil {
				return err
			}
			keys[nodeID.ReplicaID] = pk
		}
		n := node{
			ID: nodeID,
		}
		builder := consensus.NewBuilder(nodeID.ReplicaID, pk)
		var consensusModule consensus.Rules
		if !modules.GetModule(consensusName, &consensusModule) {
			return fmt.Errorf("unknown consensus module: '%s'", consensusName)
		}
		builder.Register(
			logging.New(fmt.Sprintf("r%dn%d", nodeID.ReplicaID, nodeID.NetworkID)),
			blockchain.New(),
			consensus.New(consensusModule),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(testutil.FixedTimeout(viewTimeout)),
			&configuration{
				node:    &n,
				network: network,
			},
			leaderRotation(scenario.Views),
			commandModule{commandGenerator: cg, node: &n},
		)
		n.Modules = builder.Build()
		network.Nodes[nodeID] = &n
		network.Replicas[nodeID.ReplicaID] = append(network.Replicas[nodeID.ReplicaID], &n)
	}
	return nil
}

func checkCommits(network *network) (safe bool, commits int) {
	i := 0
	for {
		noCommits := true
		commitCount := make(map[consensus.Hash]int)
		for _, replica := range network.Replicas {
			if len(replica) != 1 {
				// TODO: should we be skipping replicas with twins?
				continue
			}
			if len(replica[0].ExecutedBlocks) <= i {
				continue
			}
			commitCount[replica[0].ExecutedBlocks[i].Hash()]++
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
func (lr leaderRotation) GetLeader(view consensus.View) hotstuff.ID {
	// we start at view 1
	v := int(view) - 1
	if v >= 0 && v < len(lr) {
		return lr[v].Leader
	}
	// default to 0 (which is an invalid id)
	return 0
}

type commandGenerator struct {
	mut     sync.Mutex
	nextCmd uint64
}

func (cg *commandGenerator) next() consensus.Command {
	cg.mut.Lock()
	defer cg.mut.Unlock()
	cmd := consensus.Command(strconv.FormatUint(cg.nextCmd, 10))
	cg.nextCmd++
	return cmd
}

type commandModule struct {
	commandGenerator *commandGenerator
	node             *node
}

// Accept returns true if the replica should accept the command, false otherwise.
func (commandModule) Accept(_ consensus.Command) bool {
	return true
}

// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
// accepted in the future.
func (commandModule) Proposed(_ consensus.Command) {}

// Get returns the next command to be proposed.
// It may run until the context is cancelled.
// If no command is available, the 'ok' return value should be false.
func (cm commandModule) Get(_ context.Context) (cmd consensus.Command, ok bool) {
	return cm.commandGenerator.next(), true
}

// Exec executes the given command.
func (cm commandModule) Exec(block *consensus.Block) {
	cm.node.ExecutedBlocks = append(cm.node.ExecutedBlocks, block)
}

func (commandModule) Fork(block *consensus.Block) {}
