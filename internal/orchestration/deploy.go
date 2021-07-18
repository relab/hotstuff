package orchestration

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"path"
	"sync"

	"github.com/relab/iago"
	fs "github.com/relab/wrfs"
	"go.uber.org/multierr"
)

// Deploy deploys the hotstuff binary to a group of servers and starts a worker on the given port.
func Deploy(g iago.Group, exePath, logLevel string) (workers map[string]WorkerSession, err error) {
	w := workerSetup{
		logLevel: logLevel,
		workers:  make(map[string]WorkerSession),
	}

	// catch panics and return any errors
	defer func() {
		err, _ = recover().(error)
	}()

	// alternative error handler that does not log
	silentPanic := func(err error) {
		panic(err)
	}

	tmpDir := "hotstuff." + randString(8)

	g.Run(iago.Task{
		Name: "Create temporary directory",
		Action: iago.Func(func(ctx context.Context, host iago.Host) (err error) {
			testDir := tempDirPath(host, tmpDir)
			host.SetVar("dir", testDir)
			err = fs.MkdirAll(host.GetFS(), iago.CleanPath(tempDirPath(host, tmpDir)), 0755)
			return err
		}),
		OnError: silentPanic,
	})

	g.Run(iago.Task{
		Name: "Upload hotstuff binary",
		Action: iago.Func(func(ctx context.Context, host iago.Host) (err error) {
			dest := path.Join(tempDirPath(host, tmpDir), "hotstuff")
			host.SetVar("exe", dest)
			return iago.Upload{
				Src:  iago.P(exePath),
				Dest: iago.P(dest),
				Mode: 0755,
			}.Apply(ctx, host)
		}),
		OnError: silentPanic,
	})

	g.Run(iago.Task{
		Name:    "Start hotstuff binary",
		Action:  &w,
		OnError: silentPanic,
	})

	return w.workers, nil
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
	// worker arguments
	logLevel string

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

	err = cmd.Start(iago.Expand(host, fmt.Sprintf("%s --log-level=%s worker", iago.GetStringVar(host, "exe"), w.logLevel)))
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
