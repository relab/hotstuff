package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
)

type Core struct {
	Globals   *globals.Globals
	EventLoop *eventloop.EventLoop
	Logger    logging.Logger
}

func NewCore(
	id hotstuff.ID,
	logTag string,
	privKey hotstuff.PrivateKey,
	gOpt ...globals.GlobalOption,
) *Core {
	logger := logging.New(fmt.Sprintf("%s%d", logTag, id))
	return &Core{
		Globals:   globals.NewGlobals(id, privKey, gOpt...),
		EventLoop: eventloop.New(logger, 100),
		Logger:    logger,
	}
}
