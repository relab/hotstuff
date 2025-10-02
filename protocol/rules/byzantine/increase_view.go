// Package byzantine contains Byzantine consensus rules.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

const NameIncreaseView = "increaseView"

type IncreaseView struct {
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain
	consensus.Ruleset
}

// NewIncreaseView returns a Byzantine replica that will try to increase the view
// out of protocol in its Propose.
func NewIncreaseView(
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	rules consensus.Ruleset,
) *IncreaseView {
	return &IncreaseView{
		config:     config,
		blockchain: blockchain,
		Ruleset:    rules,
	}
}

func (iv *IncreaseView) ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	const ByzViewExtraIncrease hotstuff.View = 1000
	proposal = hotstuff.NewProposeMsg(iv.config.ID(), view+ByzViewExtraIncrease, qc, cmd) // Bump up view
	return proposal, true
}

var _ consensus.Ruleset = (*IncreaseView)(nil)
