package twins_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff/twins"
)

const jsonWant = `{
	"num_nodes": 4,
	"num_twins": 1,
	"partitions": 2,
	"rounds": 7,
	"shuffle": false,
	"seed": 0,
	"scenarios": [
		[{"leader":1,"partitions":[[1,2,3],[4,5]]}]
	]
}`

var settingsWant = twins.Settings{
	NumNodes:   4,
	NumTwins:   1,
	Partitions: 2,
	Rounds:     7,
	Shuffle:    false,
	Seed:       0,
}

var scenarioWant = twins.Scenario{
	twins.View{
		Leader: 1,
		Partitions: []twins.NodeSet{
			{1: {}, 2: {}, 3: {}},
			{4: {}, 5: {}},
		},
	},
}

func TestFromJSON(t *testing.T) {
	source, err := twins.FromJSON(bytes.NewReader([]byte(jsonWant)))
	if err != nil {
		t.Fatal(err)
	}

	if got := source.Settings(); got != settingsWant {
		t.Errorf("got: %v, want: %v", got, settingsWant)
	}

	scenario, err := source.NextScenario()
	if err != nil {
		t.Fatal(err)
	}

	if len(scenario) != 1 || scenario[0].Leader != scenarioWant[0].Leader ||
		!equalPartitions(scenario[0].Partitions, scenarioWant[0].Partitions) {

		t.Errorf("got: %v, want: %v", scenario, scenarioWant)
	}
}

func TestToJSON(t *testing.T) {
	var buf bytes.Buffer
	wr, err := twins.ToJSON(settingsWant, &buf)
	if err != nil {
		t.Fatal(err)
	}
	err = wr.WriteScenario(scenarioWant)
	if err != nil {
		t.Fatal(err)
	}
	err = wr.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != jsonWant {
		t.Errorf("got: %v, want: %v", got, jsonWant)
	}
}

func equalPartitions(a, b []twins.NodeSet) bool {
	if len(a) != len(b) {
		return false
	}

	pairsFound := 0

	for _, pa := range a {
	inner:
		for _, pb := range b {
			if len(pa) != len(pb) {
				continue
			}
			for node := range pa {
				if !pb.Contains(node) {
					continue inner
				}
			}
			pairsFound++
		}
	}

	return pairsFound == len(a)
}
