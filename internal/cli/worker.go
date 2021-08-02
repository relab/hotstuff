package cli

import (
	"bufio"
	"log"
	"os"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/modules"
	"github.com/spf13/cobra"
)

var (
	dataPath      string
	cpuProfile    string
	memProfile    string
	trace         string
	fgprofProfile string

	metrics             []string
	measurementInterval time.Duration
)

// workerCmd represents the worker command
var workerCmd = &cobra.Command{
	Hidden: true,
	Use:    "worker",
	Short:  "Run a worker.",
	Long: `Starts a worker that reads commands from stdin and writes responses to stdout.
This is only intended to be used by a controller (hotstuff run).`,
	Run: func(cmd *cobra.Command, args []string) {
		runWorker()
	},
}

func init() {
	rootCmd.AddCommand(workerCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// workerCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// workerCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	workerCmd.Flags().StringVar(&dataPath, "data-path", "", "Path to store experiment data.")
	workerCmd.Flags().StringVar(&cpuProfile, "cpu-profile", "", "Path to store a CPU profile")
	workerCmd.Flags().StringVar(&memProfile, "mem-profile", "", "Path to store a memory profile")
	workerCmd.Flags().StringVar(&trace, "trace", "", "Path to store a trace")
	workerCmd.Flags().StringVar(&fgprofProfile, "fgprof-profile", "", "Path to store a fgprof profile")

	workerCmd.Flags().StringSliceVar(&metrics, "metrics", nil, "the metrics to enable")
	workerCmd.Flags().DurationVar(&measurementInterval, "measurement-interval", 0, "the interval between measurements")
}

func runWorker() {
	stopProfilers, err := profiling.StartProfilers(cpuProfile, memProfile, trace, fgprofProfile)
	checkf("failed to start profilers: %v", err)
	defer func() {
		err = stopProfilers()
		checkf("failed to stop profilers: %v", err)
	}()

	metricsLogger := modules.NopLogger()
	if dataPath != "" {
		f, err := os.OpenFile(dataPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		checkf("failed to create data path: %v", err)
		writer := bufio.NewWriter(f)
		metricsLogger, err = modules.NewJSONLogger(writer)
		defer func() {
			err = metricsLogger.Close()
			checkf("failed to close metrics logger: %v", err)
			err = writer.Flush()
			checkf("failed to flush writer: %v", err)
			err = f.Close()
			checkf("failed to close writer: %v", err)
		}()
	}

	worker := orchestration.NewWorker(protostream.NewWriter(os.Stdout), protostream.NewReader(os.Stdin), metricsLogger, metrics, measurementInterval)
	err = worker.Run()
	if err != nil {
		log.Println(err)
	}
}
