package twins_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/twins"
)

const vulnerableModule = "vulnerableFHS"

const fhsBugScenario = `
{
	"num_nodes": 4,
	"num_twins": 0,
	"partitions": 2,
	"views": 11,
	"scenarios": [
		[
			{
				"leader": 1,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 3}, {"ReplicaID": 4}], [] ],
				"comment": "view 1"
			},
			{
				"leader": 1,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 3}, {"ReplicaID": 4}], [] ],
				"comment": "view 2"
			},
			{
				"leader": 1,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 3}, {"ReplicaID": 4}], [] ],
				"comment": "view 3"
			},
			{
				"leader": 1,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 3}, {"ReplicaID": 4}], [] ],
				"comment": "view 4. replicas time out in this view"
			},
			{
				"leader": 2,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 3}, {"ReplicaID": 4}], [{"ReplicaID": 2}] ],
				"comment": "view 5."
			},
			{
				"leader": 1,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 3}, {"ReplicaID": 4}], [{"ReplicaID": 2}] ],
				"comment": "view 6"
			},
			{
				"leader": 3,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 4}], [{"ReplicaID": 3}] ],
				"comment": "view 7"
			},
			{
				"leader": 2,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 2}, {"ReplicaID": 4}], [{"ReplicaID": 3}] ],
				"comment": "view 8"
			},
			{
				"leader": 2,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 3}, {"ReplicaID": 4}], [{"ReplicaID": 2}] ],
				"comment": "view 9"
			},
			{
				"leader": 3,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 3}, {"ReplicaID": 4}], [{"ReplicaID": 2}] ],
				"comment": "view 10"
			},
			{
				"leader": 3,
				"partitions": [ [{"ReplicaID": 1}, {"ReplicaID": 3}, {"ReplicaID": 4}], [{"ReplicaID": 2}] ],
				"comment": "view 11"
			}
		]
	]
}
`

var (
	logLevel = flag.String("log-level", "info", "set the log level")
	logAll   = flag.Bool("log-all", false, "print all logs on success")
)

func TestFHSBug(t *testing.T) {
	t.Skip("This test is not working as expected; skipping until we have fixed the issue.")
	logging.SetLogLevel(*logLevel)

	src, err := twins.FromJSON(strings.NewReader(fhsBugScenario))
	if err != nil {
		t.Fatalf("failed to read JSON: %v", err)
	}

	scenario, err := src.NextScenario()
	if err != nil {
		t.Fatalf("failed to get scenario: %v", err)
	}

	settings := src.Settings()

	res, err := twins.ExecuteScenario(scenario, settings.NumNodes, settings.NumTwins, 100, vulnerableModule)
	if err != nil {
		t.Fatalf("failed to execute scenario: %v", err)
	}

	for id, blocks := range res.NodeCommits {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Node %v commits: \n", id)
		for _, block := range blocks {
			fmt.Fprintf(&sb, "\t Proposer: %d, View: %d, Hash: %.6s\n", block.Proposer(), block.View(), block.Hash())
		}
		t.Log(sb.String())
	}

	if res.Safe {
		t.Error("expected scenario to be unsafe")
	}

	if res.Safe || *logAll {
		t.Logf("Network log:\n%s", res.NetworkLog)
		for id, log := range res.NodeLogs {
			t.Logf("Node %v log:\n%s", id, log)
		}
	}
}
