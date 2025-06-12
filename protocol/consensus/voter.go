package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/security/cert"
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
		proposal := event.(hotstuff.ProposeMsg)
		// ensure that I can vote in this view based on the protocol's rule.
		if err := v.Verify(&proposal); err != nil {
			v.logger.Infof("failed to verify incoming vote: %v", err)
			return
		}
		v.OnValidPropose(&proposal)
	})
	return v
}

// OnValidPropose is called when receiving a valid proposal from a leader and emits an event to advance the
// view. The proposal should be verified before calling this. The method tells the committer and command cache
// to update its state.
func (v *Voter) OnValidPropose(proposal *hotstuff.ProposeMsg) (hotstuff.PartialCert, error) {
	v.logger.Debug("Received proposal: %v", proposal.Block)
	block := proposal.Block
	// store the valid block, it may commit the block or its ancestors
	v.committer.Update(block)
	pc, err := v.Vote(block)
	if err != nil {
		// if the block is invalid, reject it. This means the command is also discarded.
		v.logger.Infof("%v", err)
	} else {
		// send the vote if it was successful
		v.protocol.SendVote(v.LastVote(), proposal, pc)
	}
	// advance the view regardless of vote success/failure
	newInfo := hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	v.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       v.config.ID(),
		SyncInfo: newInfo,
	})

	return pc, err
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
	// vote rule must be valid
	if !v.ruler.VoteRule(view, *proposal) {
		return fmt.Errorf("vote rule not satisfied")
	}
	// verify the proposal's QC.
	qc := proposal.Block.QuorumCert()
	if err := v.auth.VerifyAnyQC(&qc, proposal.AggregateQC); err != nil {
		return err
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
