package twins_test

import (
	"testing"
	"time"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/twins"
)

func TestTwins(t *testing.T) {
	const (
		numNodes = 4
		numTwins = 1
	)

	g := twins.NewGenerator(logging.New(""), twins.Settings{
		NumNodes:   numNodes,
		NumTwins:   numTwins,
		Partitions: 2,
		Views:      8,
	})
	seed := time.Now().Unix()
	g.Shuffle(seed)

	scenarioCount := 10
	totalCommits := 0

	for range scenarioCount {
		s, err := g.NextScenario()
		if err != nil {
			break
		}
		result, err := twins.ExecuteScenario(s, numNodes, numTwins, 100, chainedhotstuff.ModuleName)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(result.Safe, result.Commits)
		t.Log(s)
		if !result.Safe {
			t.Logf("Scenario not safe: %v", s)
			continue
		}
		if result.Commits > 0 {
			totalCommits += result.Commits
		}
	}

	t.Logf("Average %.1f commits per scenario.", float64(totalCommits)/float64(scenarioCount))
}
