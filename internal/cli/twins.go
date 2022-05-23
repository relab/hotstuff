package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/twins"
	"github.com/spf13/cobra"
)

var (
	numReplicas         uint8
	numTwins            uint8
	numPartitions       uint8
	numRounds           uint8
	numScenarios        uint64
	numScenariosPerFile uint64
	shuffle             bool
	randSeed            int64
	twinsDest           string
	twinsSrc            string
	twinsConsensus      string
	logAll              bool
	concurrency         uint
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
	twinsCmd.Flags().Uint64Var(&numScenarios, "scenarios", 0, "Number of scenarios to generate.")
	twinsCmd.Flags().Uint64Var(&numScenariosPerFile, "scenarios-per-file", 0, "Number of scenarios to write to a single file.\nIf set to 0, all scenarios will be written to a single file.")
	twinsCmd.Flags().BoolVar(&shuffle, "shuffle", false, "Shuffle the order in which scenarios are generated.")
	twinsCmd.Flags().Int64Var(&randSeed, "seed", time.Now().Unix(), "Random seed (defaults to current timestamp).")
	twinsCmd.Flags().StringVar(&twinsDest, "output", "", "If scenarios-per-file is 0, this specifies the file to write to.\nOtherwise this specifies the directory to write files to.")
	twinsCmd.Flags().StringVar(&twinsSrc, "input", "", "File to read scenarios from.")
	twinsCmd.Flags().StringVar(&twinsConsensus, "consensus", "chainedhotstuff", "The name of the consensus implementation to use.")
	twinsCmd.Flags().BoolVar(&logAll, "log-all", false, "If true, all scenarios will be written to the output file when in \"run\" mode.")
	twinsCmd.Flags().UintVar(&concurrency, "concurrency", 1, "Number of goroutines to use. If set to 0, the number of CPUs will be used.")
}

func twinsRun() {
	var (
		source twins.ScenarioSource
		err    error
	)
	if twinsSrc == "" {
		source = newGen(logging.New(""))
	} else {
		f, err := os.Open(twinsSrc)
		checkf("failed to open source file: %v", err)

		source, err = twins.FromJSON(f)
		checkf("failed to read JSON file: %v", err)
	}

	t, err := newInstance(source)
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.closeOutput()) }()

	if numScenarios == 0 {
		numScenarios = uint64(t.source.Remaining())
	}

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
	t, err := newInstance(newGen(logging.New("")))
	checkf("failed to create twins instance: %v", err)
	defer func() { checkf("failed to close twins instance: %v", t.closeOutput()) }()

	if numScenarios == 0 {
		numScenarios = uint64(t.source.Remaining())
	}

	for i := uint64(0); i < numScenarios; i++ {
		if err := t.generateAndLogScenario(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			checkf("failed to generate scenario: %v", err)
		}
	}

	log.Println("done")
}

type twinsInstance struct {
	source       twins.ScenarioSource
	outputStream scenarioWriter
	logger       logging.Logger
	closeOutput  func() error
}

func newGen(logger logging.Logger) *twins.Generator {
	gen := twins.NewGenerator(logger, numReplicas, numTwins, numPartitions, numRounds)

	if shuffle {
		gen.Shuffle(randSeed)
	}

	return gen
}

func newInstance(scenarioSource twins.ScenarioSource) (twinsInstance, error) {
	var (
		output      scenarioWriter
		closeOutput func() error
		err         error
	)
	if twinsDest != "" {
		if numScenariosPerFile == 0 {
			f, err := os.OpenFile(twinsDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				return twinsInstance{}, err
			}
			wr := bufio.NewWriter(f)
			output, err = twins.ToJSON(scenarioSource.Settings(), wr)
			if err != nil {
				_ = f.Close()
				return twinsInstance{}, err
			}
			closeOutput = func() error {
				err := output.Close()
				if ferr := wr.Flush(); err == nil {
					err = ferr
				}
				if cerr := f.Close(); err == nil {
					err = cerr
				}
				return err
			}
		} else {
			err := os.MkdirAll(twinsDest, 0755)
			if err != nil {
				return twinsInstance{}, err
			}
			output = &dirWriter{
				settings: scenarioSource.Settings(),
				dir:      twinsDest,
			}

			closeOutput = output.Close
		}
	} else {
		output, err = twins.ToJSON(scenarioSource.Settings(), io.Discard)
		if err != nil {
			return twinsInstance{}, err
		}

		closeOutput = func() error { return nil }
	}

	return twinsInstance{
		source:       scenarioSource,
		outputStream: output,
		logger:       logging.New("twins"),
		closeOutput: func() error {
			if cerr := closeOutput(); err == nil {
				err = cerr
			}
			return err
		},
	}, nil
}

func (ti twinsInstance) generateAndLogScenario() error {
	scenario, err := ti.source.NextScenario()
	if err != nil {
		return err
	}

	err = ti.outputStream.WriteScenario(scenario)
	if err != nil {
		return err
	}

	return nil
}

func (ti twinsInstance) generateAndExecuteScenario() (bool, error) {
	scenario, err := ti.source.NextScenario()
	if err != nil {
		return false, nil
	}

	t := time.Now()

	result, err := twins.ExecuteScenario(scenario, numReplicas, numTwins, twinsConsensus)
	if err != nil {
		return false, err
	}

	ti.logger.Debugf("%d commits, duration: %s", result.Commits, time.Since(t).String())

	if !result.Safe {
		ti.logger.Info("Found unsafe scenario: %v", scenario)
		fmt.Fprintln(os.Stderr, "================ Network Logs ================")
		fmt.Fprintln(os.Stderr, result.NetworkLog)

		for id, log := range result.NodeLogs {

			fmt.Fprintf(os.Stderr, "================ Node %v Logs ================\n", id)
			fmt.Fprintln(os.Stderr, log)
		}
	}

	if !result.Safe || logAll {
		err := ti.outputStream.WriteScenario(scenario)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

type scenarioWriter interface {
	WriteScenario(scenario twins.Scenario) error
	Close() error
}

type dirWriter struct {
	mut           sync.Mutex
	dir           string
	scenarioCount int
	fileCount     int
	closeFile     func() error
	settings      twins.Settings
	writer        *twins.JSONWriter
}

func (dw *dirWriter) openNewFile() error {
	f, err := os.OpenFile(filepath.Join(dw.dir, strconv.Itoa(dw.fileCount)+".json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	wr := bufio.NewWriter(f)
	dw.closeFile = func() error {
		err := wr.Flush()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
	dw.writer, err = twins.ToJSON(dw.settings, wr)
	if err != nil {
		_ = dw.closeFile()
	}
	dw.fileCount++
	return nil
}

func (dw *dirWriter) Close() error {
	dw.mut.Lock()
	defer dw.mut.Unlock()
	return dw.closeInner()
}

func (dw *dirWriter) closeInner() error {
	if dw.writer != nil {
		err := dw.writer.Close()
		dw.writer = nil
		if dw.closeFile != nil {
			if cerr := dw.closeFile(); err == nil {
				err = cerr
			}
		}
		dw.closeFile = nil
		return err
	}
	return nil
}

func (dw *dirWriter) WriteScenario(scenario twins.Scenario) error {
	dw.mut.Lock()
	defer dw.mut.Unlock()

	if dw.writer == nil {
		err := dw.openNewFile()
		if err != nil {
			return err
		}
	}

	err := dw.writer.WriteScenario(scenario)
	if err != nil {
		// I guess we'll just close the file if the write fails
		_ = dw.closeInner()
		return err
	}

	dw.scenarioCount++
	if dw.scenarioCount == int(numScenariosPerFile) {
		dw.scenarioCount = 0
		err = dw.closeInner()
		if err != nil {
			return err
		}
	}

	return nil
}
