package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

type Core struct {
	Options   *core.Options
	EventLoop *eventloop.EventLoop
	Logger    logging.Logger
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
) *Core {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &Core{
		Options:   core.NewOptions(id, privKey),
		EventLoop: eventloop.New(logger, 100),
		Logger:    logger,
	}
}
