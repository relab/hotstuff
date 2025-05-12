package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
)

type Core struct {
	globals   *globals.Globals
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
	gOpt ...globals.Option,
) *Core {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &Core{
		globals:   globals.NewGlobals(id, privKey, gOpt...),
		eventLoop: eventloop.New(logger, 100),
		logger:    logger,
	}
}

// Globals returns the global variables and configurations.
func (c *Core) Globals() *globals.Globals {
	return c.globals
}

// EventLoop returns the eventloop instance.
func (c *Core) EventLoop() *eventloop.EventLoop {
	return c.eventLoop
}

// Logger returns the logger instance.
func (c *Core) Logger() logging.Logger {
	return c.logger
}
