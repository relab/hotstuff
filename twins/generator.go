package twins

import (
	"math/rand"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type leaderPartitions struct {
	leader     uint8
	partitions []uint8
}

// Generator generates twins scenarios.
type Generator struct {
	rounds            uint8
	indices           []int
	offsets           []int
	partitions        int
	leadersPartitions []leaderPartitions
	consensusCtor     func() consensus.Consensus

	replicas []hotstuff.ID
	nodes    []NodeID
}

// NewGenerator creates a new generator.
func NewGenerator(replicas, twins, partitions, rounds uint8, consensus func() consensus.Consensus) *Generator {
	pg := newPartGen(replicas+twins, partitions)

	g := &Generator{
		rounds:        rounds,
		partitions:    int(partitions),
		indices:       make([]int, rounds),
		offsets:       make([]int, rounds),
		replicas:      make([]hotstuff.ID, 0, replicas),
		nodes:         make([]NodeID, 0, replicas+twins),
		consensusCtor: consensus,
	}

	for p := pg.nextPartitions(); p != nil; p = pg.nextPartitions() {
		for id := uint8(0); id < replicas; id++ {
			g.leadersPartitions = append(g.leadersPartitions, leaderPartitions{
				leader:     id,
				partitions: p,
			})
		}
	}

	replicaID := hotstuff.ID(1)
	networkID := uint32(1)
	remainingTwins := twins

	for i := 0; i < int(replicas); i++ {
		g.replicas = append(g.replicas, replicaID)
		g.nodes = append(g.nodes, NodeID{
			ReplicaID: replicaID,
			NetworkID: networkID,
		})
		networkID++
		if remainingTwins > 0 {
			g.nodes = append(g.nodes, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			remainingTwins--
			networkID++
		}
		replicaID++
	}

	return g
}

// Shuffle shuffles the list of leaders and partitions.
func (g *Generator) Shuffle() {
	rand.Shuffle(len(g.leadersPartitions), func(i, j int) {
		g.leadersPartitions[i], g.leadersPartitions[j] = g.leadersPartitions[j], g.leadersPartitions[i]
	})
	for i := range g.offsets {
		g.offsets[i] = rand.Intn(len(g.leadersPartitions))
	}
}

// NextScenario generates the next scenario.
func (g *Generator) NextScenario() (s Scenario, ok bool) {
	// This is basically computing the cartesian product of leadersPartitions with itself "round" times.
	p := make([]leaderPartitions, g.rounds)
	for i, ii := range g.indices {
		index := ii + g.offsets[i]
		if index >= len(g.leadersPartitions) {
			index -= len(g.leadersPartitions)
		}
		p[i] = g.leadersPartitions[index]
	}
	for i := int(g.rounds) - 1; i >= 0; i-- {
		g.indices[i]++
		if g.indices[i] < len(g.leadersPartitions) {
			break
		}
		g.indices[i] = 0
		if i <= 0 {
			g.indices = g.indices[0:0]
			return s, false
		}
	}
	s = Scenario{
		Replicas:      g.replicas,
		Nodes:         g.nodes,
		Rounds:        int(g.rounds),
		ConsensusCtor: g.consensusCtor,
		ViewTimeout:   5,
	}

	for _, partition := range p {
		s.Leaders = append(s.Leaders, g.replicas[partition.leader])
		partitions := make([]NodeSet, g.partitions)
		for i, partitionNo := range partition.partitions {
			if partitions[partitionNo] == nil {
				partitions[partitionNo] = make(NodeSet)
			}
			partitions[partitionNo].Add(g.nodes[i])
		}
		s.Partitions = append(s.Partitions, partitions)
	}

	return s, true
}

// partGen generates partitions of n nodes.
// It will not generate more than k partitions at once.
//
// algorithm based on:
// https://stackoverflow.com/questions/30893292/generate-all-partitions-of-a-set
type partGen struct {
	p    []uint8
	m    []uint8
	k    uint8
	done bool
}

func newPartGen(n, k uint8) partGen {
	return partGen{
		p: make([]uint8, n),
		m: make([]uint8, n), // m_0 = 0; m_i = max(m_(i-1), p_(i-1))
		k: k,
	}
}

func (g *partGen) nextPartitions() (partitionNumbers []uint8) {
start:
	found := false
	// calculate next state
	for i := len(g.p) - 1; i >= 0; i-- {
		if g.p[i] < g.k-1 && g.p[i] <= g.m[i] {
			g.p[i]++
			if i < len(g.p)-1 {
				g.m[i+1] = max(g.m[i], g.p[i])
			}
			found = true
			break
		}
	}

	if !found {
		if g.done {
			return nil
		}
		// return the last value one time
		g.done = true
	}

	partitionSizes := make(map[uint8]uint8)
	for _, partitionNumber := range g.p {
		partitionSizes[partitionNumber]++
	}

	majority := false
	for _, partitionSize := range partitionSizes {
		majority = partitionSize >= uint8(hotstuff.QuorumSize(len(g.p)))
	}

	// if there is no partition with at least 2f+1 nodes, the synchronizer will not be able to advance,
	// and thus there is no point in generating these scenarios.
	if !majority {
		goto start
	}

	// copy and return new state
	partitionNumbers = make([]uint8, len(g.p))
	copy(partitionNumbers, g.p)
	return partitionNumbers
}

func max(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}
