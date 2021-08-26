package cli

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/iago"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/types/known/durationpb"
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
	runCmd.Flags().Duration("max-timeout", 0, "upper limit on view timeouts")
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
	runCmd.Flags().StringSlice("byzantine", nil, "byzantine strategies to use, as a comma separated list of 'name:count'")

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}

func runController() {
	var err error
	outputDir := ""
	if output := viper.GetString("output"); output != "" {
		outputDir, err = filepath.Abs(output)
		checkf("failed to get absolute path: %v", err)
		err = os.MkdirAll(outputDir, 0755)
		checkf("failed to create output directory: %v", err)
	}

	experiment := orchestration.Experiment{
		NumReplicas: viper.GetInt("replicas"),
		NumClients:  viper.GetInt("clients"),
		Duration:    viper.GetDuration("duration"),
		ReplicaOpts: &orchestrationpb.ReplicaOpts{
			UseTLS:            true,
			BatchSize:         viper.GetUint32("batch-size"),
			TimeoutMultiplier: float32(viper.GetFloat64("timeout-multiplier")),
			Consensus:         viper.GetString("consensus"),
			Crypto:            viper.GetString("crypto"),
			LeaderRotation:    viper.GetString("leader-rotation"),
			ConnectTimeout:    durationpb.New(viper.GetDuration("connect-timeout")),
			InitialTimeout:    durationpb.New(viper.GetDuration("view-timeout")),
			TimeoutSamples:    viper.GetUint32("duration-samples"),
			MaxTimeout:        durationpb.New(viper.GetDuration("max-timeout")),
		},
		ClientOpts: &orchestrationpb.ClientOpts{
			UseTLS:           true,
			ConnectTimeout:   durationpb.New(viper.GetDuration("connect-timeout")),
			PayloadSize:      viper.GetUint32("payload-size"),
			MaxConcurrent:    viper.GetUint32("max-concurrent"),
			RateLimit:        viper.GetFloat64("rate-limit"),
			RateStep:         viper.GetFloat64("rate-step"),
			RateStepInterval: durationpb.New(viper.GetDuration("rate-step-interval")),
		},
	}

	experiment.Byzantine, err = parseByzantine()
	checkf("%v", err)

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
		worker, wait := localWorker(outputDir, viper.GetStringSlice("metrics"), viper.GetDuration("measurement-interval"))
		defer wait()
		experiment.Hosts["localhost"] = worker
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

func parseByzantine() (map[string]int, error) {
	strategies := make(map[string]int)
	byzantine := viper.GetStringSlice("byzantine")
	for _, arg := range byzantine {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("byzantine must be specified as a comma separated list of 'name:count'")
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("could not read number of replicas for byzantine strategy '%s': %w", arg, err)
		}
		strategies[parts[0]] = count
	}
	return strategies, nil
}

func localWorker(output string, metrics []string, interval time.Duration) (worker orchestration.RemoteWorker, wait func()) {
	// set up a local worker
	controllerPipe, workerPipe := net.Pipe()
	c := make(chan struct{})
	go func() {
		var logger modules.MetricsLogger
		if output != "" {
			f, err := os.OpenFile(filepath.Join(output, "measurements.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			checkf("failed to create output file: %v", err)
			defer func() { checkf("failed to close output file: %v", f.Close()) }()

			wr := bufio.NewWriter(f)
			defer func() { checkf("failed to flush writer: %v", wr.Flush()) }()

			logger, err = modules.NewJSONLogger(wr)
			checkf("failed to create JSON logger: %v", err)
			defer func() { checkf("failed to close logger: %v", logger.Close()) }()
		} else {
			logger = modules.NopLogger()
		}

		worker := orchestration.NewWorker(
			protostream.NewWriter(workerPipe),
			protostream.NewReader(workerPipe),
			logger,
			metrics,
			interval,
		)

		err := worker.Run()
		if err != nil {
			log.Fatal(err)
		}
		close(c)
	}()

	wait = func() {
		<-c
	}

	return orchestration.NewRemoteWorker(
		protostream.NewWriter(controllerPipe), protostream.NewReader(controllerPipe),
	), wait
}

func stderrPipe(r io.Reader, errChan chan<- error) {
	_, err := io.Copy(os.Stderr, r)
	errChan <- err
}
