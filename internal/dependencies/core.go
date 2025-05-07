package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
)

type DepSetCore struct {
	Options   *core.Options
	EventLoop *eventloop.EventLoop
	Logger    logging.Logger
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
) *DepSetCore {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &DepSetCore{
		Options:   core.NewOptions(id, privKey),
		EventLoop: eventloop.New(logger, 100),
		Logger:    logger,
	}
}
