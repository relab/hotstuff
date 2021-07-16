package cmd

import (
	"log"
	"os"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/spf13/cobra"
)

// workerCmd represents the worker command
var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run a worker that takes part in an experiment.",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
}

func runWorker() {
	worker := orchestration.NewWorker(protostream.NewWriter(os.Stdout), protostream.NewReader(os.Stdin))
	err := worker.Run()
	if err != nil {
		log.Println(err)
	}
}
