package proposer

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
)

type Proposer struct {
	eventLoop *eventloop.EventLoop
	config    *core.RuntimeConfig
	ruler     modules.ProposeRuler
	cmdGen    modules.CommandGenerator
}

func New(
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	cmdGen modules.CommandGenerator,
	opts ...Option,
) *Proposer {
	p := &Proposer{
		eventLoop: eventLoop,
		config:    config,
		ruler:     nil,
		cmdGen:    cmdGen,
	}
	p.ruler = p
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// CreateProposal attempts to create a new outgoing proposal if a command exists and the protocol's rule is satisfied.
func (cs *Proposer) CreateProposal(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) (proposal hotstuff.ProposeMsg, err error) {
	err = nil
	ctx, cancel := timeout.Context(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()
	// find a value to propose.
	// NOTE: this is blocking until a batch is present in the cache.
	cmd, ok := cs.cmdGen.Get(ctx)
	if !ok {
		return proposal, fmt.Errorf("no command")
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	// NOTE: the ruler will create the proposal too.
	proposal, ok = cs.ruler.ProposeRule(view, highQC, syncInfo, cmd)
	if !ok {
		return proposal, fmt.Errorf("propose rule not satisfied")
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
