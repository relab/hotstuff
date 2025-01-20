package cli

import (
	"bufio"
	"io"
	"log"
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
	"github.com/spf13/viper"
	"golang.org/x/exp/rand"
)

func runController() {
	cfg, err := config.NewViper()
	checkf("viper config error: %v", err)

	cuePath := viper.GetString("cue")
	if cuePath != "" {
		cfg, err = config.NewCue(cuePath, cfg)
		checkf("config error when loading %s: %v", cuePath, err)
	}

	// A cleaner way would be to have this in the constructor of
	// config, but this is done here since the Cue might overwrite
	// RandomTree.
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

	// TODO: Generate DeployConfig from ExperimentConfig type.
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
