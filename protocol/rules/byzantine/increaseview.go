package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
)

const NameIncreaseView = "increaseview"

type IncreaseView struct {
	config *core.RuntimeConfig
	consensus.Ruleset
}

// NewIncreaseView returns a repliaca that proposes with an inflated view number.
func NewIncreaseView(
	config *core.RuntimeConfig,
	rules consensus.Ruleset,
) *IncreaseView {
	return &IncreaseView{
		config:  config,
		Ruleset: rules,
	}
}

func (iv *IncreaseView) ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	const ByzViewExtraIncrease hotstuff.View = 1000
	proposal = hotstuff.NewProposeMsg(iv.config.ID(), view+ByzViewExtraIncrease, qc, cmd)
	return proposal, true
}

var _ consensus.Ruleset = (*IncreaseView)(nil)
