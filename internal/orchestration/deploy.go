package orchestration

import (
	"fmt"
	"os"

	"github.com/relab/iago"
)

// Deploy deploys the hotstuff binary to a group of servers and starts a worker on the given port.
func Deploy(g iago.Group, exePath, port, logLevel string) (err error) {
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

	go func() {
		g.Run(iago.Task{
			Name: "Start hotstuff binary",
			Action: iago.Shell{
				Command: fmt.Sprintf("env HOTSTUFF_LOG=%s $HOME/hotstuff worker %s", logLevel, port),
				Stdout:  os.Stdout,
				Stderr:  os.Stderr,
			},
			OnError: silentPanic,
		})
	}()

	return nil
}
