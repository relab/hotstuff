package acceptor

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/certauth"
)

type Acceptor struct {
	logger    logging.Logger
	eventLoop *eventloop.EventLoop
	config    *core.RuntimeConfig

	leaderRotation modules.LeaderRotation
	rules          modules.ConsensusRules

	auth *certauth.CertAuthority

	lastVote hotstuff.View
}

// TODO(AlanRostem): finish up this class.
func New(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	leaderRotation modules.LeaderRotation,
	rules modules.ConsensusRules,
	auth *certauth.CertAuthority,
) *Acceptor {
	return &Acceptor{
		logger:    logger,
		eventLoop: eventLoop,
		config:    config,

		leaderRotation: leaderRotation,
		rules:          rules,

		auth: auth,

		lastVote: 0,
	}
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (v *Acceptor) StopVoting(view hotstuff.View) {
	if v.lastVote < view {
		v.logger.Debugf("stopped voting on view %d and changed view to %d", v.lastVote, view)
		v.lastVote = view
	}
}

// Vote votes for and signs the block, returning a partial certificate
// if the vote was successful.
func (v *Acceptor) Vote(block *hotstuff.Block) (pc hotstuff.PartialCert, ok bool) {
	ok = false
	// cannot vote for an old block.
	if block.View() <= v.lastVote {
		v.logger.Info("TryAccept: block view too old")
		return
	}
	// try to sign the block. Abort if this fails.
	pc, err := v.auth.CreatePartialCert(block)
	if err != nil {
		v.logger.Error("OnPropose: failed to sign block: ", err)
		return
	}
	// block is safe, so we update the view we voted for
	// i.e., we voted for this block!
	v.lastVote = block.View()
	return pc, true
}

// TryAccept verifies the proposal and returns true if it can be voted for.
func (v *Acceptor) TryAccept(proposal *hotstuff.ProposeMsg) (accepted bool) {
	block := proposal.Block
	view := block.View()
	if !v.rules.VoteRule(view, *proposal) {
		v.logger.Info("TryAccept: Block not voted for")
		return
	}
	accepted = false
	// verify the proposal's QC.
	qc := proposal.Block.QuorumCert()
	if !v.auth.VerifyAnyQC(&qc, proposal.AggregateQC) {
		return
	}
	// ensure the block came from the expected leader.
	if proposal.ID != v.leaderRotation.GetLeader(block.View()) {
		v.logger.Infof("TryAccept[view=%d]: block was not proposed by the expected leader", view)
		return
	}
	return true
}

func (v *Acceptor) LastVote() hotstuff.View {
	return v.lastVote
}
