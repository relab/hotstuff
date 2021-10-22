package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto/twinspb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/twins"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	numReplicas    uint8
	numTwins       uint8
	numPartitions  uint8
	numRounds      uint8
	numScenarios   uint64
	shuffle        bool
	randSeed       int64
	twinsDest      string
	twinsConsensus string
	twinsTimeout   time.Duration
	logAll         bool
)

var twinsCmd = &cobra.Command{
	Use:   "twins [run|generate]",
	Short: "Generate and execute Twins scenarios.",
	Long:  `The twins command allows for generating and executing twins scenarios.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			err := cmd.Usage()
			if err != nil {
				return err
			}
		}

		switch args[0] {
		case "run":
			twinsRun()
		case "generate":
			twinsGenerate()
		default:
			return fmt.Errorf("unknown argument '%s'", args[0])
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(twinsCmd)

	twinsCmd.Flags().Uint8Var(&numReplicas, "replicas", 4, "Number of replicas.")
	twinsCmd.Flags().Uint8Var(&numTwins, "twins", 1, "Number of \"evil\" twins.")
	twinsCmd.Flags().Uint8Var(&numPartitions, "partitions", 2, "Number of network partitions.")
	twinsCmd.Flags().Uint8Var(&numRounds, "rounds", 7, "Number of rounds in each scenario.")
	twinsCmd.Flags().Uint64Var(&numScenarios, "scenarios", 100, "Number of scenarios to generate.")
	twinsCmd.Flags().BoolVar(&shuffle, "shuffle", false, "Shuffle the order in which scenarios are generated.")
	twinsCmd.Flags().Int64Var(&randSeed, "seed", 0, "Random seed (defaults to current timestamp).")
	twinsCmd.Flags().StringVar(&twinsDest, "output", "twins.json", "File to write to.")
	twinsCmd.Flags().StringVar(&twinsConsensus, "consensus", "chainedhotstuff", "The name of the consensus implementation to use.")
	twinsCmd.Flags().DurationVar(&twinsTimeout, "timeout", 10*time.Millisecond, "View timeout.")
	twinsCmd.Flags().BoolVar(&logAll, "log-all", false, "If true, all scenarios will be written to the output file when in \"run\" mode.")
}

func twinsRun() {
	t, err := newInstance()
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.close()) }()

	for i := uint64(0); i < numScenarios; i++ {
		if ok, err := t.generateAndExecuteScenario(); err != nil {
			checkf("failed to execute scenario: %v", err)
		} else if !ok {
			break
		}
	}

	log.Println("done")
}

func twinsGenerate() {
	t, err := newInstance()
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.close()) }()

	for i := uint64(0); i < numScenarios; i++ {
		if !t.generateAndLogScenario() {
			break
		}
	}

	log.Println("done")
}

type twinsInstance struct {
	generator   *twins.Generator
	mods        *modules.Modules
	closeOutput func() error
}

func newInstance() (twinsInstance, error) {
	f, err := os.OpenFile(twinsDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return twinsInstance{}, err
	}

	gen := twins.NewGenerator(numReplicas, numTwins, numPartitions, numRounds)

	if shuffle {
		gen.Shuffle(randSeed)
	}

	wr := bufio.NewWriter(f)
	jsonLogger, err := modules.NewJSONLogger(wr)
	if err != nil {
		if cerr := f.Close(); err == nil {
			return twinsInstance{}, cerr
		}
		return twinsInstance{}, err
	}

	jsonLogger.Log(&twinspb.GeneratorSettings{
		Replicas:    uint32(numReplicas),
		Twins:       uint32(numTwins),
		Rounds:      uint32(numRounds),
		Shuffle:     shuffle,
		Seed:        randSeed,
		Consensus:   twinsConsensus,
		ViewTimeout: durationpb.New(twinsTimeout),
	})

	mods := modules.NewBuilder(0)
	mods.Register(
		logging.New("twins"),
		jsonLogger,
	)

	return twinsInstance{
		generator: gen,
		mods:      mods.Build(),
		closeOutput: func() error {
			err = wr.Flush()
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			return err
		},
	}, nil
}

func (ti twinsInstance) close() error {
	err := ti.mods.MetricsLogger().Close()
	if cerr := ti.closeOutput(); err == nil {
		err = cerr
	}
	return err
}

func (ti twinsInstance) generateAndLogScenario() bool {
	scenario, ok := ti.generator.NextScenario()
	if !ok {
		return false
	}

	ti.mods.MetricsLogger().Log(twins.ScenarioToProto(&scenario))

	return true
}

func (ti twinsInstance) generateAndExecuteScenario() (bool, error) {
	scenario, ok := ti.generator.NextScenario()
	if !ok {
		return false, nil
	}

	safe, commits, err := twins.ExecuteScenario(scenario, twinsConsensus, twinsTimeout)
	if err != nil {
		return false, err
	}

	if !safe {
		ti.mods.Logger().Info("Found unsafe scenario: %v", scenario)
	}

	if !safe || logAll {
		ti.mods.MetricsLogger().Log(&twinspb.Result{
			Safe:     safe,
			Commits:  uint32(commits),
			Scenario: twins.ScenarioToProto(&scenario),
		})
	}

	return true, nil
}
