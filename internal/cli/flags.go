package cli

import (
	"math"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an experiment.",
	Long: `The run command runs an experiment locally or on remote workers.
By default, a local experiment is run.
To run experiments on remote machines, you must create a 'ssh_config' file (or use ~/.ssh/config).
This should at the very least specify the identity file to use.
It is also required that the host keys for all remote machines are present in a 'known_hosts' file.
Then, you must use the '--ssh-config' parameter to specify the location of your 'ssh_config' file
(or omit it to use ~/.ssh/config). Then, you must specify the list of remote machines to connect to
using the '--host' parameter. This should be a comma separated list of hostnames or ip addresses.`,
	Run: func(_ *cobra.Command, _ []string) {
		runController()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().Int("replicas", 4, "number of replicas to run")
	runCmd.Flags().Int("clients", 1, "number of clients to run")
	runCmd.Flags().Int("batch-size", 1, "number of commands to batch together in each block")
	runCmd.Flags().Int("payload-size", 0, "size in bytes of the command payload")
	runCmd.Flags().Int("max-concurrent", 4, "maximum number of concurrent commands per client")
	runCmd.Flags().Duration("client-timeout", 500*time.Millisecond, "Client timeout.")
	runCmd.Flags().Duration("duration", 10*time.Second, "duration of the experiment")
	runCmd.Flags().Duration("connect-timeout", 5*time.Second, "duration of the initial connection timeout")
	// need longer default timeout for the kauri
	runCmd.Flags().Duration("view-timeout", 500*time.Millisecond, "duration of the first view")
	runCmd.Flags().Duration("max-timeout", 0, "upper limit on view timeouts")
	runCmd.Flags().Int("duration-samples", 1000, "number of previous views to consider when predicting view duration")
	runCmd.Flags().Float32("timeout-multiplier", 1.2, "number to multiply the view duration by in case of a timeout")
	runCmd.Flags().String("consensus", "chainedhotstuff", "name of the consensus implementation")
	runCmd.Flags().String("crypto", "ecdsa", "name of the crypto implementation")
	runCmd.Flags().String("leader-rotation", "round-robin", "name of the leader rotation algorithm")
	runCmd.Flags().Int64("shared-seed", 0, "Shared random number generator seed")

	runCmd.Flags().Bool("worker", false, "run a local worker")
	runCmd.Flags().StringSlice("replica-hosts", nil, "the remote hosts to run replicas on via ssh")
	runCmd.Flags().StringSlice("client-hosts", nil, "the remote hosts to run clients on via ssh")
	runCmd.Flags().String("exe", "", "path to the executable to deploy and run on remote workers")
	runCmd.Flags().String("ssh-config", "", "path to ssh_config file to resolve host aliases (defaults to ~/.ssh/config)")

	runCmd.Flags().String("output", "", "the directory to save data and profiles to (disabled by default)")
	runCmd.Flags().Bool("cpu-profile", false, "enable cpu profiling")
	runCmd.Flags().Bool("mem-profile", false, "enable memory profiling")
	runCmd.Flags().Bool("trace", false, "enable trace")
	runCmd.Flags().Bool("fgprof-profile", false, "enable fgprof")

	runCmd.Flags().StringSlice("metrics", []string{"client-latency", "throughput"}, "list of metrics to enable")
	runCmd.Flags().Duration("measurement-interval", 0, "time interval between measurements")
	runCmd.Flags().Float64("rate-limit", math.Inf(1), "rate limit for clients (in commands/second)")
	runCmd.Flags().Float64("rate-step", 0, "rate limit step up for clients (in commands/second)")
	runCmd.Flags().Duration("rate-step-interval", time.Hour, "how often the client rate limit should be increased")
	// tree config parameter
	runCmd.Flags().Int("bf", 2, "branch factor of the tree")
	runCmd.Flags().IntSlice("tree-pos", []int{}, "tree positions of the replicas")
	runCmd.Flags().Duration("tree-delta", 30*time.Millisecond, "waiting time for intermediate nodes in the tree")
	runCmd.Flags().Bool("random-tree", false, "randomize tree positions, tree-pos is ignored if set")

	runCmd.Flags().String("cue", "", "Cue-based host config")
	runCmd.Flags().Bool("kauri", false, "enable Kauri protocol")
	runCmd.Flags().Bool("agg-qc", false, "enable QC aggregation")

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}
