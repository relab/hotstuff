package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
)

const NameSilence = "silence"

type Silence struct {
	consensus.Ruleset
}

// NewSilence returns a Byzantine replica that will never propose.
func NewSilence(rules consensus.Ruleset) *Silence {
	return &Silence{Ruleset: rules}
}

func (s *Silence) ProposeRule(_ hotstuff.View, _ hotstuff.QuorumCert, _ hotstuff.SyncInfo, _ *clientpb.Batch) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

var _ consensus.Ruleset = (*Silence)(nil)
