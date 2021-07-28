package cmd

import (
	"bufio"
	"log"
	"os"

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
}

func runWorker() {
	stopProfilers, err := profiling.StartProfilers(cpuProfile, memProfile, trace, fgprofProfile)
	if err != nil {
		log.Fatalln("failed to start profilers: ", err)
	}
	defer func() {
		err = stopProfilers()
		if err != nil {
			log.Fatalln("failed to stop profilers: ", err)
		}
	}()

	dataLogger := modules.NopLogger()
	if dataPath != "" {
		f, err := os.OpenFile(dataPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalln("failed to create data path: ", err)
		}
		dataLogger = modules.NewDataLogger(protostream.NewWriter(bufio.NewWriter(f)))
		defer func() {
			err = f.Close()
			if err != nil {
				log.Fatalln("failed to close data logger: ", err)
			}
		}()
	}

	worker := orchestration.NewWorker(protostream.NewWriter(os.Stdout), protostream.NewReader(os.Stdin), dataLogger)
	err = worker.Run()
	if err != nil {
		log.Println(err)
	}
}
