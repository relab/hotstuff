package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

type Core struct {
	config    *core.RuntimeConfig
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
	gOpt ...core.RuntimeOption,
) *Core {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &Core{
		config:    core.NewRuntimeConfig(id, privKey, gOpt...),
		eventLoop: eventloop.New(logger, 100),
		logger:    logger,
	}
}

// Globals returns the global variables and configurations.
func (c *Core) Globals() *core.RuntimeConfig {
	return c.config
}

// EventLoop returns the eventloop instance.
func (c *Core) EventLoop() *eventloop.EventLoop {
	return c.eventLoop
}

// Logger returns the logger instance.
func (c *Core) Logger() logging.Logger {
	return c.logger
}
