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
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/iago"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/rand"
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
	outputDir := ""
	if output := viper.GetString("output"); output != "" {
		outputDir, err = filepath.Abs(output)
		checkf("failed to get absolute path: %v", err)
		err = os.MkdirAll(outputDir, 0o755)
		checkf("failed to create output directory: %v", err)
	}

	numReplicas := viper.GetInt("replicas")
	numClients := viper.GetInt("clients")

	// Kauri values
	branchFactor := viper.GetUint32("bf")

	cfgPath := viper.GetString("cue")

	treeDelta := viper.GetDuration("tree-delta")
	intTreePos := viper.GetIntSlice("tree-pos")

	replicaHosts := viper.GetStringSlice("replica-hosts")
	clientHosts := viper.GetStringSlice("client-hosts")

	var cfg *config.HostConfig
	if cfgPath != "" {
		cfg, err = config.Load(cfgPath)
		checkf("config error when loading %s: %v", cfgPath, err)

		// TODO: Find a better approach to overwrite the cli flags.

		treeDelta = cfg.TreeDelta
		intTreePos = nil
		for _, id := range cfg.TreePositions {
			intTreePos = append(intTreePos, int(id))
		}
		branchFactor = cfg.BranchFactor

		replicaHosts = cfg.ReplicaHosts
		clientHosts = cfg.ClientHosts
	} else {
		// If a config is not specified, use the user/default cli flags
		// and instantiate a config for a local run.
		cfg = config.NewLocal(numReplicas, numClients)

		// If the hosts are specified in cli, overwrite the local cfg.

		cfg = &config.HostConfig{
			Replicas: numReplicas,
			Clients:  numClients,
		}
		if len(replicaHosts) == 0 {
			cfg.ReplicaHosts = []string{"localhost"}
		} else {
			cfg.ReplicaHosts = replicaHosts
		}

		if len(clientHosts) == 0 {
			cfg.ClientHosts = []string{"localhost"}
		} else {
			cfg.ClientHosts = clientHosts
		}
	}

	// If the treePos is empty, generate them by the replica count.
	var treePos []uint32
	if len(intTreePos) == 0 {
		treePos = tree.DefaultTreePosUint32(numReplicas)
	} else {
		treePos = make([]uint32, len(intTreePos))
		for i, pos := range intTreePos {
			treePos[i] = uint32(pos)
		}
	}
	if viper.GetBool("random-tree") {
		rnd := rand.New(rand.NewSource(rand.Uint64()))
		rnd.Shuffle(len(treePos), reflect.Swapper(treePos))
	}

	worker := viper.GetBool("worker")
	exePath := viper.GetString("exe")

	allHosts := append(replicaHosts, clientHosts...)

	g, err := iago.NewSSHGroup(allHosts, viper.GetString("ssh-config"))
	checkf("failed to connect to remote hosts: %v", err)

	if exePath == "" {
		exePath, err = os.Executable()
		checkf("failed to get executable path: %v", err)
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
	checkf("failed to deploy workers: %v", err)

	errors := make(chan error)

	remoteWorkers := make(map[string]orchestration.RemoteWorker)

	for host, session := range sessions {
		remoteWorkers[host] = orchestration.NewRemoteWorker(
			protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()),
		)
		go stderrPipe(session.Stderr(), errors)
	}

	if worker || len(allHosts) == 0 {
		worker, wait := localWorker(outputDir, viper.GetStringSlice("metrics"), viper.GetDuration("measurement-interval"))
		defer wait()
		remoteWorkers["localhost"] = worker
	}

	replicaOpts := &orchestrationpb.ReplicaOpts{
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
		SharedSeed:        viper.GetInt64("shared-seed"),
		Modules:           viper.GetStringSlice("modules"),
		TreePositions:     treePos,
		BranchFactor:      branchFactor,
		TreeDelta:         durationpb.New(treeDelta),
	}

	clientOpts := &orchestrationpb.ClientOpts{
		UseTLS:           true,
		ConnectTimeout:   durationpb.New(viper.GetDuration("connect-timeout")),
		PayloadSize:      viper.GetUint32("payload-size"),
		MaxConcurrent:    viper.GetUint32("max-concurrent"),
		RateLimit:        viper.GetFloat64("rate-limit"),
		RateStep:         viper.GetFloat64("rate-step"),
		RateStepInterval: durationpb.New(viper.GetDuration("rate-step-interval")),
		Timeout:          durationpb.New(viper.GetDuration("client-timeout")),
	}

	experiment, err := orchestration.NewExperiment(
		viper.GetDuration("duration"),
		outputDir,
		replicaOpts,
		clientOpts,
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

	err = orchestration.FetchData(g, outputDir)
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
