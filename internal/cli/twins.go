package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/proto/twinspb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/twins"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	logAll         bool
	concurrency    uint
)

var twinsCmd = &cobra.Command{
	Use:   "twins [run|generate]",
	Short: "Generate and execute Twins scenarios.",
	Long:  `The twins command allows for generating and executing twins scenarios.`,

	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// set the log-level for the consensus package to error by default, unless it was changed.
		flag := viper.GetStringSlice("log-pkgs")
		for _, v := range flag {
			if strings.Contains(v, "consensus") {
				return
			}
		}
		logging.SetPackageLogLevel("consensus", "panic")
	},

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
	twinsCmd.Flags().BoolVar(&logAll, "log-all", false, "If true, all scenarios will be written to the output file when in \"run\" mode.")
	twinsCmd.Flags().UintVar(&concurrency, "concurrency", 1, "Number of worker goroutines to use for executing scenarios. If set to 0, it will create one goroutine for each CPU.")
}

func twinsRun() {
	t, err := newInstance()
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.closeOutput()) }()

	var wg sync.WaitGroup

	numWorkers := concurrency
	if concurrency == 0 {
		numWorkers = uint(runtime.NumCPU())
	}

	wg.Add(int(numWorkers))

	for i := 0; i < int(numWorkers); i++ {
		go func() {
			for i := uint64(0); i < numScenarios/uint64(numWorkers); i++ {
				if ok, err := t.generateAndExecuteScenario(); err != nil {
					checkf("failed to execute scenario: %v", err)
				} else if !ok {
					break
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()

	log.Println("done")
}

func twinsGenerate() {
	t, err := newInstance()
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.closeOutput()) }()

	for i := uint64(0); i < numScenarios; i++ {
		if ok, err := t.generateAndLogScenario(); err != nil {
			checkf("failed to generate scenario: %v", err)
		} else if !ok {
			break
		}
	}

	log.Println("done")
}

type twinsInstance struct {
	generator    *twins.Generator
	outputStream *protostream.Writer
	logger       logging.Logger
	closeOutput  func() error
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
	ps := protostream.NewWriter(wr)

	err = ps.Write(&twinspb.GeneratorSettings{
		Replicas:  uint32(numReplicas),
		Twins:     uint32(numTwins),
		Rounds:    uint32(numRounds),
		Shuffle:   shuffle,
		Seed:      randSeed,
		Consensus: twinsConsensus,
	})
	if err != nil {
		return twinsInstance{}, err
	}

	return twinsInstance{
		generator:    gen,
		outputStream: ps,
		logger:       logging.New("twins"),
		closeOutput: func() error {
			err = wr.Flush()
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			return err
		},
	}, nil
}

func (ti twinsInstance) generateAndLogScenario() (bool, error) {
	scenario, ok := ti.generator.NextScenario()
	if !ok {
		return false, nil
	}

	err := ti.outputStream.Write(twins.ScenarioToProto(&scenario))
	if err != nil {
		return false, err
	}

	return true, nil
}

func (ti twinsInstance) generateAndExecuteScenario() (bool, error) {
	scenario, ok := ti.generator.NextScenario()
	if !ok {
		return false, nil
	}

	t := time.Now()

	safe, commits, err := twins.ExecuteScenario(scenario, twinsConsensus)
	if err != nil {
		return false, err
	}

	ti.logger.Debugf("%d commits, duration: %s", commits, time.Since(t).String())

	if !safe {
		ti.logger.Info("Found unsafe scenario: %v", scenario)
	}

	if !safe || logAll {
		err = ti.outputStream.Write(&twinspb.Result{
			Safe:     safe,
			Commits:  uint32(commits),
			Scenario: twins.ScenarioToProto(&scenario),
		})
		if err != nil {
			return false, err
		}
	}

	return true, nil
}
