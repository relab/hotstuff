// Package orchestration implements deployment and orchestration of hotstuff replicas and clients on remote hosts.
// A "controller" uses the iago framework to connect to hosts via ssh and deploy hotstuff.
// This is implemented by the Deploy() function.
// The deploy function also starts a "worker" on each connected host.
// The controller communicates via workers through the worker's stdin and stdout streams.
// The worker's stderr stream is forwarded to the stderr stream of the controller.
package orchestration

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/relab/iago"
	fs "github.com/relab/wrfs"
	"go.uber.org/multierr"
)

// DeployConfig contains configuration options for deployment.
type DeployConfig struct {
	ExePath             string
	LogLevel            string
	CPUProfiling        bool
	MemProfiling        bool
	Tracing             bool
	Fgprof              bool
	Metrics             []string
	MeasurementInterval time.Duration
}

// Deploy deploys the hotstuff binary to a group of servers and starts a worker on the given port.
func Deploy(g iago.Group, cfg DeployConfig) (workers map[string]WorkerSession, err error) {
	w := workerSetup{
		cfg:     cfg,
		workers: make(map[string]WorkerSession),
	}

	exe, err := filepath.Abs(cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("failed to make ExePath absolute: %w", err)
	}

	// catch panics and return any errors
	defer func() {
		err, _ = recover().(error)
	}()

	// alternative error handler that does not log
	g.ErrorHandler = func(err error) {
		panic(err)
	}

	g.Run("Create temporary directory",
		func(ctx context.Context, host iago.Host) (err error) {
			tmpDir := "hotstuff." + randString(8)
			testDir := strings.TrimPrefix(tempDirPath(host, tmpDir), "/")
			dataDir := testDir + "/data"
			host.SetVar("test-dir", testDir)
			host.SetVar("data-dir", dataDir)
			err = fs.MkdirAll(host.GetFS(), dataDir, 0755)
			return err
		})

	g.Run(
		"Upload hotstuff binary",
		func(ctx context.Context, host iago.Host) (err error) {
			dest, err := iago.NewPath("/", iago.GetStringVar(host, "test-dir")+"/hotstuff")
			if err != nil {
				return err
			}
			host.SetVar("exe", dest.String())
			src, err := iago.NewPathFromAbs(exe)
			if err != nil {
				return err
			}
			return iago.Upload{
				Src:  src,
				Dest: dest,
				Perm: iago.NewPerm(0755),
			}.Apply(ctx, host)
		})

	g.Run("Start hotstuff binary", w.Apply)

	return w.workers, nil
}

// FetchData downloads the data from the workers.
func FetchData(g iago.Group, dest string) (err error) {
	// catch panics and return any errors
	defer func() {
		err, _ = recover().(error)
	}()

	// alternative error handler that does not log
	g.ErrorHandler = func(err error) {
		panic(err)
	}

	if dest != "" {
		g.Run("Download test data",
			func(ctx context.Context, host iago.Host) (err error) {
				src, err := iago.NewPath("/", iago.GetStringVar(host, "data-dir")) // assuming the dir variable was set earlier
				if err != nil {
					return err
				}
				dst, err := iago.NewPathFromAbs(dest)
				if err != nil {
					return err
				}
				return iago.Download{
					Src:  src,
					Dest: dst,
				}.Apply(ctx, host)
			})
	}

	g.Run("Remove test directory",
		func(ctx context.Context, host iago.Host) (err error) {
			err = fs.RemoveAll(host.GetFS(), iago.GetStringVar(host, "test-dir"))
			return err
		})

	return nil
}

// WorkerSession contains the state of a connected worker.
type WorkerSession struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	cmd    iago.CmdRunner
}

// Stdin returns a writer to the the worker's stdin stream.
func (ws WorkerSession) Stdin() io.Writer {
	return ws.stdin
}

// Stdout returns a reader of the worker's stdout stream.
func (ws WorkerSession) Stdout() io.Reader {
	return ws.stdout
}

// Stderr returns a reader of the worker's stderr stream.
func (ws WorkerSession) Stderr() io.Reader {
	return ws.stderr
}

// Close closes the session and all of its streams.
func (ws WorkerSession) Close() (err error) {
	err = multierr.Append(err, ws.cmd.Wait())
	// apparently, closing the streams can return EOF, so we'll have to check for that.
	if cerr := ws.stdin.Close(); cerr != nil && cerr != io.EOF {
		err = multierr.Append(err, cerr)
	}
	if cerr := ws.stdout.Close(); cerr != nil && cerr != io.EOF {
		err = multierr.Append(err, cerr)
	}
	if cerr := ws.stderr.Close(); cerr != nil && cerr != io.EOF {
		err = multierr.Append(err, cerr)
	}
	return
}

type workerSetup struct {
	cfg DeployConfig

	mut     sync.Mutex
	workers map[string]WorkerSession
}

func (w *workerSetup) Apply(ctx context.Context, host iago.Host) (err error) {
	cmd, err := host.NewCommand()
	if err != nil {
		return err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	dir := "/" + iago.GetStringVar(host, "data-dir")

	var sb strings.Builder
	sb.WriteString(iago.GetStringVar(host, "exe"))
	sb.WriteString(" ")

	sb.WriteString("--data-path ")
	sb.WriteString(path.Join(dir, "measurements.json"))
	sb.WriteString(" ")

	if w.cfg.MeasurementInterval > 0 {
		sb.WriteString("--measurement-interval ")
		sb.WriteString(w.cfg.MeasurementInterval.String())
		sb.WriteString(" ")
	}

	sb.WriteString("--metrics=\"")
	for _, metric := range w.cfg.Metrics {
		sb.WriteString(metric)
		sb.WriteString(",")
	}
	sb.WriteString("\" ")

	if w.cfg.CPUProfiling {
		sb.WriteString("--cpu-profile ")
		sb.WriteString(path.Join(dir, "cpuprofile"))
		sb.WriteString(" ")
	}
	if w.cfg.MemProfiling {
		sb.WriteString("--mem-profile ")
		sb.WriteString(path.Join(dir, "memprofile"))
		sb.WriteString(" ")
	}
	if w.cfg.Tracing {
		sb.WriteString("--trace ")
		sb.WriteString(path.Join(dir, "trace"))
		sb.WriteString(" ")
	}
	if w.cfg.Fgprof {
		sb.WriteString("--fgprof-profile ")
		sb.WriteString(path.Join(dir, "fgprofprofile"))
		sb.WriteString(" ")
	}
	sb.WriteString("--log-level ")
	sb.WriteString(w.cfg.LogLevel)
	sb.WriteString(" worker")

	err = cmd.Start(iago.Expand(host, sb.String()))
	if err != nil {
		return err
	}

	w.mut.Lock()
	w.workers[host.Name()] = WorkerSession{stdin, stdout, stderr, cmd}
	w.mut.Unlock()

	return nil
}

func tempDirPath(host iago.Host, dirName string) string {
	tmp := host.GetEnv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	return path.Join(tmp, dirName)
}

func randString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}
