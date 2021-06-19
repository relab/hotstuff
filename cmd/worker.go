package cmd

import (
	"log"
	"net"
	"strconv"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
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
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatalln("port must be a number")
		}
		runWorker(port)
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

func runWorker(port int) {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", port, err)
	}
	srv := gorums.NewServer()
	w := orchestration.NewWorker()
	orchestrationpb.RegisterOrchestratorServer(srv, w)
	err = srv.Serve(lis)
	if err != nil {
		log.Fatalln(err)
	}
}
