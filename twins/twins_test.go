package twins_test

import (
	"testing"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/twins"
)

func TestTwins(t *testing.T) {
	g := twins.NewGenerator(4, 1, 2, 8, func() consensus.Consensus {
		return consensus.New(chainedhotstuff.New())
	})
	g.Shuffle()

	for i := 0; i < 100; i++ {
		s, ok := g.NextScenario()
		if !ok {
			break
		}
		safe, commits, err := twins.ExecuteScenario(s)
		if err != nil {
			t.Fatal(err)
		}
		if !safe {
			t.Logf("Scenario not safe: %v", s)
			continue
		}
		if commits > 0 {
			t.Logf("Scenario did commit: %v", s)
			continue
		}
	}
}
