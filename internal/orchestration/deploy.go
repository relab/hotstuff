package orchestration

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/relab/iago"
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

	g.Run(iago.Task{
		Name: "Upload hotstuff binary",
		Action: iago.Upload{
			Src:  iago.P(exePath),
			Dest: iago.P("$HOME/hotstuff"),
			Mode: 0755,
		},
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

	err = cmd.Start(iago.Expand(host, fmt.Sprintf("env HOTSTUFF_LOG=%s $HOME/hotstuff worker", w.logLevel)))
	if err != nil {
		return err
	}

	w.mut.Lock()
	w.workers[host.Name()] = WorkerSession{stdin, stdout, stderr, cmd}
	w.mut.Unlock()

	return nil
}
