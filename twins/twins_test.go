package twins_test

import (
	"testing"
	"time"

	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/twins"
)

func TestTwins(t *testing.T) {
	const (
		numNodes = 4
		numTwins = 1
	)

	g := twins.NewGenerator(logging.New(""), numNodes, numTwins, 2, 7)
	g.Shuffle(time.Now().Unix())

	scenarios := 10
	totalCommits := 0

	for i := 0; i < scenarios; i++ {
		s, err := g.NextScenario()
		if err != nil {
			break
		}
		result, err := twins.ExecuteScenario(s, numNodes, numTwins, "chainedhotstuff")
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

	t.Logf("Average %f commits per scenario.", float64(totalCommits)/float64(scenarios))
}
