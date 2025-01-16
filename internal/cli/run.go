package cli

import (
	"bufio"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/iago"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/rand"
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
	runCmd.Flags().StringSlice("modules", nil, "Name additional modules to be loaded.")

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
	runCmd.Flags().StringSlice("byzantine", nil, "byzantine strategies to use, as a comma separated list of 'name:count'")
	// tree config parameter
	runCmd.Flags().Int("bf", 2, "branch factor of the tree")
	runCmd.Flags().IntSlice("tree-pos", []int{}, "tree positions of the replicas")
	runCmd.Flags().Duration("tree-delta", 30*time.Millisecond, "waiting time for intermediate nodes in the tree")
	runCmd.Flags().Bool("random-tree", false, "randomize tree positions, tree-pos is ignored if set")

	runCmd.Flags().String("cue", "", "Cue-based host config")

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}

func runController() {
	var err error

	cfgPath := viper.GetString("cue")
	var cfg *config.HostConfig
	if cfgPath != "" {
		cfg, err = config.NewCue(cfgPath)
		checkf("config error when loading %s: %v", cfgPath, err)
	} else {
		cfg, err = config.NewViper()
		checkf("viper config error: %v", err)
	}

	if cfg.RandomTree {
		rnd := rand.New(rand.NewSource(rand.Uint64()))
		rnd.Shuffle(len(cfg.TreePositions), reflect.Swapper(cfg.TreePositions))
	}

	// If the config is set to run locally, `hosts` will be nil (empty)
	// and when passed to iago.NewSSHGroup, thus iago will not generate
	// an SSH group.
	var hosts []string
	if !cfg.IsLocal() {
		hosts = cfg.AllHosts()
	}

	g, err := iago.NewSSHGroup(hosts, cfg.SshConfig)
	checkf("failed to connect to remote hosts: %v", err)

	if cfg.Exe == "" {
		cfg.Exe, err = os.Executable()
		checkf("failed to get executable path: %v", err)
	}

	// TODO: Generate DeployConfig from HostsConfig type.
	sessions, err := orchestration.Deploy(g, orchestration.DeployConfig{
		ExePath:             cfg.Exe,
		LogLevel:            cfg.LogLevel,
		CPUProfiling:        cfg.CpuProfile,
		MemProfiling:        cfg.MemProfile,
		Tracing:             cfg.Trace,
		Fgprof:              cfg.FgProfProfile,
		Metrics:             cfg.Metrics,
		MeasurementInterval: cfg.MeasurementInterval,
	})
	checkf("failed to deploy workers: %v", err)

	errors := make(chan error)

	remoteWorkers := make(map[string]orchestration.RemoteWorker)

	for host, session := range sessions {
		remoteWorkers[host] = orchestration.NewRemoteWorker(
			protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()),
		)
		go stderrPipe(session.Stderr(), errors)
	}

	if cfg.Worker || len(hosts) == 0 {
		worker, wait := localWorker(cfg.Output, cfg.Metrics, cfg.MeasurementInterval)
		defer wait()
		remoteWorkers["localhost"] = worker
	}

	experiment, err := orchestration.NewExperiment(
		cfg,
		remoteWorkers,
		logging.New("ctrl"),
	)

	checkf("config error: %v", err)

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

	err = orchestration.FetchData(g, cfg.Output)
	checkf("failed to fetch data: %v", err)

	err = g.Close()
	checkf("failed to close ssh connections: %v", err)
}

func checkf(format string, args ...any) {
	for _, arg := range args {
		if err, _ := arg.(error); err != nil {
			log.Fatalf(format, args...)
		}
	}
}

func localWorker(globalOutput string, enableMetrics []string, interval time.Duration) (worker orchestration.RemoteWorker, wait func()) {
	// set up an output dir
	output := ""
	if globalOutput != "" {
		output = filepath.Join(globalOutput, "local")
		err := os.MkdirAll(output, 0o755)
		checkf("failed to create local output directory: %v", err)
	}

	// start profiling
	stopProfilers, err := startLocalProfiling(output)
	checkf("failed to start local profiling: %v", err)

	// set up a local worker
	controllerPipe, workerPipe := net.Pipe()
	c := make(chan struct{})
	go func() {
		var logger metrics.Logger
		if output != "" {
			f, err := os.OpenFile(filepath.Join(output, "measurements.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			checkf("failed to create output file: %v", err)
			defer func() { checkf("failed to close output file: %v", f.Close()) }()

			wr := bufio.NewWriter(f)
			defer func() { checkf("failed to flush writer: %v", wr.Flush()) }()

			logger, err = metrics.NewJSONLogger(wr)
			checkf("failed to create JSON logger: %v", err)
			defer func() { checkf("failed to close logger: %v", logger.Close()) }()
		} else {
			logger = metrics.NopLogger()
		}

		worker := orchestration.NewWorker(
			protostream.NewWriter(workerPipe),
			protostream.NewReader(workerPipe),
			logger,
			enableMetrics,
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
		checkf("failed to stop local profilers: %v", stopProfilers())
	}

	return orchestration.NewRemoteWorker(
		protostream.NewWriter(controllerPipe), protostream.NewReader(controllerPipe),
	), wait
}

func stderrPipe(r io.Reader, errChan chan<- error) {
	_, err := io.Copy(os.Stderr, r)
	errChan <- err
}

func startLocalProfiling(output string) (stop func() error, err error) {
	var (
		cpuProfile    string
		memProfile    string
		trace         string
		fgprofProfile string
	)

	if output == "" {
		return func() error { return nil }, nil
	}

	if viper.GetBool("cpu-profile") {
		cpuProfile = filepath.Join(output, "cpuprofile")
	}

	if viper.GetBool("mem-profile") {
		memProfile = filepath.Join(output, "memprofile")
	}

	if viper.GetBool("trace") {
		trace = filepath.Join(output, "trace")
	}

	if viper.GetBool("fgprof-profile") {
		fgprofProfile = filepath.Join(output, "fgprofprofile")
	}

	stop, err = profiling.StartProfilers(cpuProfile, memProfile, trace, fgprofProfile)
	return
}
