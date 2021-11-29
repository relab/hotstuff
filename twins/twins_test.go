package twins_test

import (
	"testing"
	"time"

	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/twins"
)

func TestTwins(t *testing.T) {
	const (
		numNodes = 4
		numTwins = 1
	)

	g := twins.NewGenerator(numNodes, numTwins, 2, 7)
	g.Shuffle(time.Now().Unix())

	scenarios := 10
	totalCommits := 0

	for i := 0; i < scenarios; i++ {
		s, err := g.NextScenario()
		if err != nil {
			break
		}
		safe, commits, err := twins.ExecuteScenario(s, numNodes, numTwins, "chainedhotstuff")
		if err != nil {
			t.Fatal(err)
		}
		t.Log(safe, commits)
		t.Log(s)
		if !safe {
			t.Logf("Scenario not safe: %v", s)
			continue
		}
		if commits > 0 {
			totalCommits += commits
		}
	}

	t.Logf("Average %f commits per scenario.", float64(totalCommits)/float64(scenarios))
}
