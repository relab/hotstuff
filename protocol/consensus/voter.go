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

// OnValidPropose is called when receiving a valid proposal from a leader and emits
// an event to advance the view. The proposal must be verified before calling this.
func (v *Voter) OnValidPropose(proposal *hotstuff.ProposeMsg) (errs error) {
	block := proposal.Block
	// try to commit the block; accumulate any error but still proceed to vote
	if err := v.committer.TryCommit(block); err != nil {
		errs = errors.Join(errs, err)
	}
	// always vote, even if commit failed
	pc, err := v.Vote(block)
	if err != nil {
		return errors.Join(errs, fmt.Errorf("vote failed: %w", err))
	}
	// send the vote if it was successful
	if err := v.aggregator.Aggregate(v.LastVote(), proposal, pc); err != nil {
		return errors.Join(errs, fmt.Errorf("aggregate failed: %w", err))
	}
	return errs
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
