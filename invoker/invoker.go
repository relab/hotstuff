package invoker

import (
	"context"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/convert"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/synctools"
)

type Invoker struct {
	configuration *netconfig.Config
	eventLoop     *core.EventLoop
	logger        logging.Logger
}

func NewInvoker() *Invoker {
	return &Invoker{}
}

func (inv *Invoker) InitModule(mods *core.Core) {
	mods.Get(
		&inv.configuration,
		&inv.eventLoop,
		&inv.logger,
	)
}

// Propose sends the block to all replicas in the configuration
func (inv *Invoker) Propose(proposal hotstuff.ProposeMsg) {
	cfg := inv.configuration.GetPbConfig()
	ctx, cancel := synctools.TimeoutContext(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()
	cfg.Propose(
		ctx,
		convert.ProposalToProto(proposal),
	)
}

// Timeout sends the timeout message to all replicas.
func (inv *Invoker) Timeout(msg hotstuff.TimeoutMsg) {
	cfg := inv.configuration.GetPbConfig()

	// will wait until the second timeout before canceling
	ctx, cancel := synctools.TimeoutContext(inv.eventLoop.Context(), inv.eventLoop)
	defer cancel()

	cfg.Timeout(
		ctx,
		convert.TimeoutMsgToProto(msg),
	)
}

// Fetch requests a block from all the replicas in the configuration
func (inv *Invoker) Fetch(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	cfg := inv.configuration.GetPbConfig()

	protoBlock, err := cfg.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			inv.logger.Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return convert.BlockFromProto(protoBlock), true
}
