package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
)

const NameSilentProposer = "silentproposer"

type SilentProposer struct {
	consensus.Ruleset
}

// NewSilentProposer returns a Byzantine replica that will never propose.
// Note: A silent proposer will still participate in voting and other
// protocol activities, it just won't propose new blocks.
func NewSilentProposer(rules consensus.Ruleset) *SilentProposer {
	return &SilentProposer{Ruleset: rules}
}

func (s *SilentProposer) ProposeRule(_ hotstuff.View, _ hotstuff.QuorumCert, _ hotstuff.SyncInfo, _ *clientpb.Batch) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

var _ consensus.Ruleset = (*SilentProposer)(nil)
