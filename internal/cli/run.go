package cli

import (
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/iago"
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

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}

func runController() {
	outputDir, err := filepath.Abs(viper.GetString("output"))
	checkf("failed to get absolute path: %v", err)
	err = os.MkdirAll(outputDir, 0755)
	checkf("failed to create output directory: %v", err)

	experiment := orchestration.Experiment{
		NumReplicas:         viper.GetInt("replicas"),
		NumClients:          viper.GetInt("clients"),
		BatchSize:           viper.GetInt("batch-size"),
		PayloadSize:         viper.GetInt("payload-size"),
		MaxConcurrent:       viper.GetInt("max-concurrent"),
		Duration:            viper.GetDuration("duration"),
		ConnectTimeout:      viper.GetDuration("connect-timeout"),
		ViewTimeout:         viper.GetDuration("view-timeout"),
		TimoutSamples:       viper.GetInt("duration-samples"),
		TimeoutMultiplier:   float32(viper.GetFloat64("timeout-multiplier")),
		Consensus:           viper.GetString("consensus"),
		Crypto:              viper.GetString("crypto"),
		LeaderRotation:      viper.GetString("leader-rotation"),
		Metrics:             viper.GetStringSlice("metrics"),
		MeasurementInterval: viper.GetDuration("measurement-interval"),
		RateLimit:           viper.GetFloat64("rate-limit"),
		RateStep:            viper.GetFloat64("rate-step"),
		RateStepInterval:    viper.GetDuration("rate-step-interval"),
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

	sessions, err := orchestration.Deploy(g, orchestration.DeployConfig{
		ExePath:             exePath,
		LogLevel:            viper.GetString("log-level"),
		CPUProfiling:        viper.GetBool("cpu-profile"),
		MemProfiling:        viper.GetBool("mem-profile"),
		Tracing:             viper.GetBool("trace"),
		Fgprof:              viper.GetBool("fgprof-profile"),
		Metrics:             viper.GetStringSlice("metrics"),
		MeasurementInterval: viper.GetDuration("measurement-interval"),
	})
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

	err = orchestration.FetchData(g, outputDir)
	checkf("failed to fetch data: %v", err)

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
		worker := orchestration.NewWorker(
			protostream.NewWriter(workerPipe),
			protostream.NewReader(workerPipe),
			modules.NopLogger(),
			viper.GetStringSlice("metrics"),
			viper.GetDuration("measurement-interval"),
		)
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
