package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/service/cmdcache"
)

type Voter struct {
	logger    logging.Logger
	eventLoop *eventloop.EventLoop
	config    *core.RuntimeConfig

	leaderRotation modules.LeaderRotation

	blockChain *blockchain.BlockChain
	auth       *certauth.CertAuthority

	commandCache *cmdcache.Cache

	lastVote hotstuff.View
}

// TODO(AlanRostem): finish up this class.
func NewVoter(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	leaderRotation modules.LeaderRotation,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
) *Voter {
	return &Voter{
		logger:    logger,
		eventLoop: eventLoop,
		config:    config,

		leaderRotation: leaderRotation,

		blockChain: blockChain,
		auth:       auth,

		lastVote: 0,
	}
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (v *Voter) StopVoting(view hotstuff.View) {
	if v.lastVote < view {
		v.logger.Debugf("stopped voting on view %d and changed view to %d", v.lastVote, view)
		v.lastVote = view
	}
}

func (v *Voter) CreateVote(block *hotstuff.Block) (pc hotstuff.PartialCert, ok bool) {
	ok = false
	// if the given block is too old, reject it.
	// TODO(AlanRostem): is this not already checked with impl.VoteRule()?
	if block.View() <= v.lastVote {
		v.logger.Info("OnPropose: block view too old")
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

func (cs *Voter) TryAccept(proposal *hotstuff.ProposeMsg) (accepted bool) {
	block := proposal.Block
	view := block.View()
	accepted = false
	// verify the proposal's QC.
	if !cs.auth.VerifyProposal(proposal) {
		return
	}
	// ensure the block came from the expected leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Infof("TryAccept[p=%d, view=%d]: block was not proposed by the expected leader", view)
		return
	}
	return true
}

func (v *Voter) LastVote() hotstuff.View {
	return v.lastVote
}
