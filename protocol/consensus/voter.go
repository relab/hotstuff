package consensus

import (
	"errors"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/cert"
)

type Voter struct {
	config *core.RuntimeConfig

	leaderRotation modules.LeaderRotation
	ruler          modules.VoteRuler
	aggregator     modules.Aggregator

	auth      *cert.Authority
	committer *Committer

	lastVote hotstuff.View
}

func NewVoter(
	config *core.RuntimeConfig,
	leaderRotation modules.LeaderRotation,
	rules modules.VoteRuler,
	aggregator modules.Aggregator,
	auth *cert.Authority,
	committer *Committer,
) *Voter {
	v := &Voter{
		config: config,

		leaderRotation: leaderRotation,
		ruler:          rules,
		aggregator:     aggregator,

		auth:      auth,
		committer: committer,

		lastVote: 0,
	}
	return v
}

// OnValidPropose is called when receiving a valid proposal from a leader and emits an event to advance the
// view. The proposal should be verified before calling this.
func (v *Voter) OnValidPropose(proposal *hotstuff.ProposeMsg) (errs error) {
	block := proposal.Block
	// store the valid block, it may commit the block or its ancestors
	if err := v.committer.TryCommit(block); err != nil {
		errs = errors.Join(fmt.Errorf("Failed to commit: %v", err))
		// want to vote for the block which is why we dont return here and join errs instead
	}
	pc, err := v.Vote(block)
	if err != nil {
		errs = errors.Join(fmt.Errorf("Rejected invalid block: %v", err))
		return
	}
	// send the vote if it was successful
	if err := v.aggregator.Aggregate(v.LastVote(), proposal, pc); err != nil {
		errs = errors.Join(err)
		return
	}
	return
}

// StopVoting ensures that no voting happens in a view earlier than `view`.
func (v *Voter) StopVoting(view hotstuff.View) {
	if v.lastVote < view {
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

// Verify verifies the proposal and returns true if it can be voted for.
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
