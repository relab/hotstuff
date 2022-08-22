package twins_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/fasthotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/twins"
)

func init() {
	modules.RegisterModule(vulnerableModule, func() consensus.Rules { return &vulnerableFHS{} })
}

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
				"partitions": [ [1, 2, 3, 4], [] ],
				"comment": "view 1"
			},
			{
				"leader": 1,
				"partitions": [ [1, 2, 3, 4], [] ],
				"comment": "view 2"
			},
			{
				"leader": 1,
				"partitions": [ [1, 2, 3, 4], [] ],
				"comment": "view 3"
			},
			{
				"leader": 1,
				"partitions": [ [1, 2, 3, 4], [] ],
				"comment": "view 4. replicas time out in this view"
			},
			{
				"leader": 2,
				"partitions": [ [1, 3, 4], [2] ],
				"comment": "view 5."
			},
			{
				"leader": 1,
				"partitions": [ [1, 3, 4], [2] ],
				"comment": "view 6"
			},
			{
				"leader": 3,
				"partitions": [ [1, 2, 4], [3] ],
				"comment": "view 7"
			},
			{
				"leader": 2,
				"partitions": [ [1, 2, 4], [3] ],
				"comment": "view 8"
			},
			{
				"leader": 2,
				"partitions": [ [1, 3, 4], [2] ],
				"comment": "view 9"
			},
			{
				"leader": 3,
				"partitions": [ [1, 3, 4], [2] ],
				"comment": "view 10"
			},
			{
				"leader": 3,
				"partitions": [ [1, 3, 4], [2] ],
				"comment": "view 11"
			}
		]
	]
}
`

var logLevel = flag.String("log-level", "info", "set the log level")
var logAll = flag.Bool("log-all", false, "print all logs on success")

func TestFHSBug(t *testing.T) {
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

// A wrapper around the FHS rules that swaps the commit rule for a vulnerable version
type vulnerableFHS struct {
	logger     logging.Logger
	blockChain modules.BlockChain
	inner      fasthotstuff.FastHotStuff
}

func (fhs *vulnerableFHS) InitModule(mods *modules.Core) {
	mods.Get(
		&fhs.logger,
		&fhs.blockChain,
	)

	fhs.inner.InitModule(mods)
}

// VoteRule decides whether to vote for the block.
func (fhs *vulnerableFHS) VoteRule(proposal hotstuff.ProposeMsg) bool {
	return fhs.inner.VoteRule(proposal)
}

func (fhs *vulnerableFHS) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.blockChain.Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block can be committed.
func (fhs *vulnerableFHS) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}
	fhs.logger.Debug("PRECOMMIT: ", parent)
	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return nil
	}
	// NOTE: this does check for a direct link between the block and the grandparent.
	// This is what causes the safety violation.
	if block.Parent() == parent.Hash() && parent.Parent() == grandparent.Hash() {
		fhs.logger.Debug("COMMIT(vulnerable): ", grandparent)
		return grandparent
	}
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (fhs *vulnerableFHS) ChainLength() int {
	return fhs.inner.ChainLength()
}
