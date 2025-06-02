package wiring

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

type Core struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
	opts ...core.RuntimeOption,
) *Core {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &Core{
		config:    core.NewRuntimeConfig(id, privKey, opts...),
		eventLoop: eventloop.New(logger, 100),
		logger:    logger,
	}
}

// EventLoop returns the eventloop instance.
func (c *Core) EventLoop() *eventloop.EventLoop {
	return c.eventLoop
}

// Logger returns the logger instance.
func (c *Core) Logger() logging.Logger {
	return c.logger
}

// RuntimeCfg returns the runtime configuration.
func (c *Core) RuntimeCfg() *core.RuntimeConfig {
	return c.config
}
