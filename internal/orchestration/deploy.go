package orchestration

import (
	"fmt"

	"github.com/Raytar/iago"
)

// Deploy deploys the hotstuff binary to a group of servers and starts a worker on the given port.
func Deploy(g iago.Group, exePath, port string) (err error) {
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
		Action:  iago.Shell(fmt.Sprintf("nohup $HOME/hotstuff worker --listen %s &", port)),
		OnError: silentPanic,
	})

	return nil
}
