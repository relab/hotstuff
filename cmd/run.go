package cmd

import (
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/iago"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an experiment.",
	Long:  `The run command runs an experiment locally or on remote workers.`,
	Run: func(cmd *cobra.Command, args []string) {
		runController()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().Int("replicas", 4, "number of replicas to run")
	runCmd.Flags().Int("clients", 1, "number of clients to run")
	runCmd.Flags().Int("batch-size", 1, "number of commands to batch together in each block")
	runCmd.Flags().Int("payload-size", 0, "size in bytes of the command payload")
	runCmd.Flags().Int("max-concurrent", 4, "maximum number of conccurrent commands per client")
	runCmd.Flags().Duration("duration", 10*time.Second, "duration of the experiment")
	runCmd.Flags().Duration("connect-timeout", 5*time.Second, "duration of the initial connection timeout")
	runCmd.Flags().Duration("view-timeout", 100*time.Millisecond, "duration of the first view")
	runCmd.Flags().Int("duration-samples", 1000, "number of previous views to consider when predicting view duration")
	runCmd.Flags().Float32("timeout-multiplier", 1.2, "number to multiply the view duration by in case of a timeout")
	runCmd.Flags().String("consensus", "chainedhotstuff", "name of the consensus implementation")
	runCmd.Flags().String("crypto", "ecdsa", "name of the crypto implementation")
	runCmd.Flags().String("leader-rotation", "round-robin", "name of the leader rotation algorithm")

	runCmd.Flags().Bool("worker", false, "run a local worker")
	runCmd.Flags().StringSlice("hosts", nil, "the remote hosts to run the experiment on via ssh")
	runCmd.Flags().String("exe", "", "path to the executable to deploy and run on remote workers")
	runCmd.Flags().String("ssh-config", "", "path to ssh_config file to resolve host aliases (defaults to ~/.ssh/config)")

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}

func runController() {
	experiment := orchestration.Experiment{
		NumReplicas:       viper.GetInt("replicas"),
		NumClients:        viper.GetInt("clients"),
		BatchSize:         viper.GetInt("batch-size"),
		PayloadSize:       viper.GetInt("payload-size"),
		MaxConcurrent:     viper.GetInt("max-concurrent"),
		Duration:          viper.GetDuration("duration"),
		ConnectTimeout:    viper.GetDuration("connect-timeout"),
		ViewTimeout:       viper.GetDuration("view-timeout"),
		TimoutSamples:     viper.GetInt("duration-samples"),
		TimeoutMultiplier: float32(viper.GetFloat64("timeout-multiplier")),
		Consensus:         viper.GetString("consensus"),
		Crypto:            viper.GetString("crypto"),
		LeaderRotation:    viper.GetString("leader-rotation"),
	}

	worker := viper.GetBool("worker")
	hosts := viper.GetStringSlice("hosts")
	exePath := viper.GetString("exe")

	g, err := iago.NewSSHGroup(hosts, viper.GetString("ssh-config"))
	checkf("Failed to connect to remote hosts: %v", err)

	if exePath == "" {
		exePath, err = os.Executable()
		checkf("Failed to get executable path: %v", err)
	}

	sessions, err := orchestration.Deploy(g, exePath, viper.GetString("log-level"))
	checkf("Failed to deploy workers: %v", err)

	errors := make(chan error)

	experiment.Hosts = make(map[string]orchestration.RemoteWorker)

	for host, session := range sessions {
		experiment.Hosts[host] = orchestration.NewRemoteWorker(
			protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()),
		)
		go stderrPipe(session.Stderr(), errors)
	}

	if worker || len(hosts) == 0 {
		experiment.Hosts["localhost"] = localWorker()
	}

	experiment.HostConfigs = make(map[string]orchestration.HostConfig)

	var hostConfigs []struct {
		Name     string
		Clients  int
		Replicas int
	}

	err = viper.UnmarshalKey("hosts-config", &hostConfigs)
	checkf("failed to unmarshal hosts-config: %v", err)

	for _, cfg := range hostConfigs {
		experiment.HostConfigs[cfg.Name] = orchestration.HostConfig{Replicas: cfg.Replicas, Clients: cfg.Clients}
	}

	err = experiment.Run()
	checkf("failed to run experiment: %v", err)

	for _, session := range sessions {
		err := session.Close()
		checkf("failed to close ssh command session: %v", err)
	}

	for range sessions {
		err = <-errors
		checkf("failed to read from remote's standard error stream %v", err)
	}

	err = g.Close()
	checkf("failed to close ssh connections: %v", err)
}

func checkf(format string, args ...interface{}) {
	for _, arg := range args {
		if err, _ := arg.(error); err != nil {
			log.Fatalf(format, args...)
		}
	}
}

func localWorker() orchestration.RemoteWorker {
	// set up a local worker
	controllerPipe, workerPipe := net.Pipe()
	go func() {
		// TODO: replace the NopLogger with a proper logger.
		worker := orchestration.NewWorker(protostream.NewWriter(workerPipe), protostream.NewReader(workerPipe), consensus.NopLogger())
		err := worker.Run()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return orchestration.NewRemoteWorker(protostream.NewWriter(controllerPipe), protostream.NewReader(controllerPipe))
}

func stderrPipe(r io.Reader, errChan chan<- error) {
	_, err := io.Copy(os.Stderr, r)
	errChan <- err
}
