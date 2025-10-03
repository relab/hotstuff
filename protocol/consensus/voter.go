package consensus

import (
	"errors"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/security/cert"
)

type Voter struct {
	config *core.RuntimeConfig

	leaderRotation leaderrotation.LeaderRotation
	ruler          VoteRuler
	aggregator     comm.Aggregator

	auth      *cert.Authority
	committer *Committer

	lastVote hotstuff.View
}

func NewVoter(
	config *core.RuntimeConfig,
	leaderRotation leaderrotation.LeaderRotation,
	rules VoteRuler,
	aggregator comm.Aggregator,
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
	if err := v.aggregator.Aggregate(proposal, pc); err != nil {
		return errors.Join(errs, fmt.Errorf("aggregate failed (view %d): %w", block.View(), err))
	}
	return errs
}

// StopVoting prevents voting in the given view and all earlier views.
// Returns true if the last vote was advanced, and false if voting had
// already stopped for this view.
func (v *Voter) StopVoting(view hotstuff.View) bool {
	if v.lastVote < view {
		v.lastVote = view
		return true
	}
	return false
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
		return fmt.Errorf("block view %d too old, last vote was %d", block.View(), v.lastVote)
	}
	// vote rule must be valid
	if !v.ruler.VoteRule(view, *proposal) {
		return fmt.Errorf("vote rule not satisfied")
	}
	// verify the proposal's quorum certificate(s).
	if err := v.auth.VerifyAnyQC(proposal); err != nil {
		return err
	}
	// ensure the block came from the expected leader.
	leaderID := v.leaderRotation.GetLeader(block.View())
	if proposal.ID != leaderID {
		return fmt.Errorf("expected leader %d but got %d in view %d", leaderID, proposal.ID, block.View())
	}
	return nil
}
