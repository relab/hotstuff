package proposer

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
	"github.com/relab/hotstuff/service/cmdcache"
)

type Proposer struct {
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	config       *core.RuntimeConfig
	ruler        modules.ProposeRuler
	commandCache *cmdcache.Cache
}

func New(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	commandCache *cmdcache.Cache,
	opts ...Option,
) *Proposer {
	p := &Proposer{
		eventLoop:    eventLoop,
		logger:       logger,
		config:       config,
		ruler:        nil,
		commandCache: commandCache,
	}
	p.ruler = p
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// CreateProposal attempts to create a new outgoing proposal if a command exists and the protocol's rule is satisfied.
func (cs *Proposer) CreateProposal(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) (proposal hotstuff.ProposeMsg, ok bool) {
	ok = false
	cs.logger.Debugf("Propose")
	ctx, cancel := timeout.Context(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()
	// find a value to propose.
	// NOTE: this is blocking until a batch is present in the cache.
	cmd, ok := cs.commandCache.Get(ctx)
	if !ok {
		cs.logger.Debugf("Propose[view=%d]: No command", view)
		return
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	proposal, ok = cs.ruler.ProposeRule(view, highQC, syncInfo, cmd)
	if !ok {
		cs.logger.Debug("Propose: No block")
		return
	}
	return
}

// ProposeRule implements the default propose ruler.
func (cs *Proposer) ProposeRule(view hotstuff.View, _ hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	proposal = hotstuff.ProposeMsg{
		ID: cs.config.ID(),
		Block: hotstuff.NewBlock(
			qc.BlockHash(),
			qc,
			cmd,
			view,
			cs.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); ok && cs.config.HasAggregateQC() {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

var _ modules.ProposeRuler = (*Proposer)(nil)
