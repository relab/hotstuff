package twins

import (
	"context"
	"strconv"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/synchronizer"
)

type Scenario struct {
	Replicas      []hotstuff.ID
	Leaders       []hotstuff.ID
	Nodes         []NodeID
	Partitions    [][]NodeSet
	Rounds        int
	ConsensusCtor func() consensus.Consensus
	ViewTimeout   float64
}

func ExecuteScenario(scenario Scenario) (err error) {
	network := Network{
		Nodes:      make(map[NodeID]*Node),
		Replicas:   make(map[hotstuff.ID][]*Node),
		Partitions: scenario.Partitions,
	}

	ctx, cancel := context.WithCancel(context.Background())

	err = createAndStartNodes(ctx, scenario, &network)
	if err != nil {
		cancel()
		return err
	}

	network.WaitUntilHung()
	cancel()

	// check if the majority of replicas have committed the same blocks
	// TODO
	isSafe(&network)

	return nil
}

func createAndStartNodes(ctx context.Context, scenario Scenario, network *Network) error {
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
		node := Node{
			ID: nodeID,
		}
		builder := consensus.NewBuilder(nodeID.ReplicaID, pk)
		builder.Register(
			blockchain.New(),
			scenario.ConsensusCtor(),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(testutil.FixedTimeout(scenario.ViewTimeout)),
			&configuration{
				node:    &node,
				network: network,
			},
			leaderRotation(scenario.Leaders),
			cg,
		)
		node.Modules = builder.Build()
		network.Nodes[nodeID] = &node
		network.Replicas[nodeID.ReplicaID] = append(network.Replicas[nodeID.ReplicaID], &node)
	}
	for _, node := range network.Nodes {
		go node.Modules.Run(ctx)
	}
	return nil
}

func isSafe(network *Network) bool {
	return false
}

type leaderRotation []hotstuff.ID

// GetLeader returns the id of the leader in the given view.
func (lr leaderRotation) GetLeader(view consensus.View) hotstuff.ID {
	// we start at view 1
	v := int(view) - 1
	if v > 0 && v < len(lr) {
		return lr[v]
	}
	// default to 0 (which is an invalid id)
	return 0
}

type commandGenerator struct {
	mut     sync.Mutex
	nextCmd uint64
}

// Accept returns true if the replica should accept the command, false otherwise.
func (commandGenerator) Accept(_ consensus.Command) bool {
	return true
}

// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
// accepted in the future.
func (commandGenerator) Proposed(_ consensus.Command) {}

// Get returns the next command to be proposed.
// It may run until the context is cancelled.
// If no command is available, the 'ok' return value should be false.
func (cg *commandGenerator) Get(_ context.Context) (cmd consensus.Command, ok bool) {
	cg.mut.Lock()
	defer cg.mut.Unlock()
	cmd = consensus.Command(strconv.FormatUint(cg.nextCmd, 10))
	cg.nextCmd++
	return cmd, true
}

// Exec executes the given command.
func (commandGenerator) Exec(_ consensus.Command) {}

func (commandGenerator) Fork(_ consensus.Command) {}
