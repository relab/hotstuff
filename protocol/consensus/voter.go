package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/service/committer"
)

type Voter struct {
	logger    logging.Logger
	eventLoop *eventloop.EventLoop
	config    *core.RuntimeConfig

	leaderRotation modules.LeaderRotation
	ruler          modules.VoteRuler
	protocol       modules.ConsensusProtocol

	auth         *cert.Authority
	commandCache *clientpb.Cache
	committer    *committer.Committer

	lastVote hotstuff.View
}

func NewVoter(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	leaderRotation modules.LeaderRotation,
	rules modules.VoteRuler,
	protocol modules.ConsensusProtocol,
	auth *cert.Authority,
	commandCache *clientpb.Cache,
	committer *committer.Committer,
) *Voter {
	v := &Voter{
		logger:    logger,
		eventLoop: eventLoop,
		config:    config,

		leaderRotation: leaderRotation,
		ruler:          rules,
		protocol:       protocol,

		auth:         auth,
		commandCache: commandCache,
		committer:    committer,

		lastVote: 0,
	}
	v.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		p := event.(hotstuff.ProposeMsg)
		v.OnPropose(&p)
	})
	return v
}

// OnPropose is called when receiving a proposal from a leader and returns true if the proposal was voted for.
func (cs *Voter) OnPropose(proposal *hotstuff.ProposeMsg) {
	block := proposal.Block
	// ensure that I can vote in this view based on the protocol's rule.
	err := cs.Verify(proposal)
	if err != nil {
		cs.logger.Infof("failed to verify incoming vote: %v", err)
		return
	}
	// store the valid block, it may commit the block or its ancestors
	cs.committer.Update(block)
	// TODO(AlanRostem): solve issue #191
	// update the command's age before voting.
	cs.commandCache.Proposed(block.Command())
	pc, err := cs.Vote(block)
	if err != nil {
		// if the block is invalid, reject it. This means the command is also discarded.
		cs.logger.Infof("%v", err)
	} else {
		// send the vote if it was successful
		cs.protocol.SendVote(proposal, pc)
	}
	// advance the view regardless of vote success/failure
	newInfo := hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       cs.config.ID(),
		SyncInfo: newInfo,
	})
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (v *Voter) StopVoting(view hotstuff.View) {
	if v.lastVote < view {
		v.logger.Debugf("stopped voting on view %d and changed view to %d", v.lastVote, view)
		v.lastVote = view
	}
}

// Vote votes for and signs the block, returning a partial certificate
// and updates the last vote view if the signature was successful.
func (v *Voter) Vote(block *hotstuff.Block) (pc hotstuff.PartialCert, err error) {
	// try to sign the block. Abort if this fails.
	pc, err = v.auth.CreatePartialCert(block)
	if err != nil {
		return pc, err
	}
	// block is safe, so we update the view we voted for
	// i.e., we voted for this block!
	v.lastVote = block.View()
	return pc, nil
}

// verify verifies the proposal and returns true if it can be voted for.
func (v *Voter) Verify(proposal *hotstuff.ProposeMsg) (err error) {
	block := proposal.Block
	view := block.View()
	// cannot vote for an old block.
	if block.View() <= v.lastVote {
		return fmt.Errorf("block view too old")
	}
	// cannot vote for old commands
	if !v.commandCache.Accept(block.Command()) {
		return fmt.Errorf("command too old")
	}
	// vote rule must be valid
	if !v.ruler.VoteRule(view, *proposal) {
		return fmt.Errorf("vote rule not satisfied")
	}
	// verify the proposal's QC.
	qc := proposal.Block.QuorumCert()
	if !v.auth.VerifyAnyQC(&qc, proposal.AggregateQC) {
		return fmt.Errorf("invalid qc")
	}
	// ensure the block came from the expected leader.
	if proposal.ID != v.leaderRotation.GetLeader(block.View()) {
		return fmt.Errorf("unexpected leader")
	}
	return nil
}

func (v *Voter) LastVote() hotstuff.View {
	return v.lastVote
}
