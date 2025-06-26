package protocol

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

// ViewStates is a shared object which stores the protocol's state and may be modified
// by several consensus component objects.
type ViewStates struct {
	blockchain *blockchain.Blockchain
	auth       *cert.Authority

	mut sync.RWMutex // to protect the following

	highTC         hotstuff.TimeoutCert
	highQC         hotstuff.QuorumCert
	view           hotstuff.View
	committedBlock *hotstuff.Block
}

func NewViewStates(
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
) (*ViewStates, error) {
	s := &ViewStates{
		blockchain: blockchain,
		auth:       auth,

		committedBlock: hotstuff.GetGenesis(),
		view:           1,
	}
	var err error
	s.highQC, err = s.auth.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		return nil, fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err)
	}
	s.highTC, err = s.auth.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		return nil, fmt.Errorf("unable to create empty timeout cert for view 0: %v", err)
	}
	return s, nil
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) error {
	newBlock, ok := s.blockchain.Get(qc.BlockHash())
	if !ok {
		return fmt.Errorf("could not find block referenced by new qc")
	}
	if newBlock.View() > s.highQC.View() {
		s.highQC = qc
	}
	return nil
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
func (s *ViewStates) UpdateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
	}
}

func (s *ViewStates) HighQC() hotstuff.QuorumCert {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.highQC
}

func (s *ViewStates) HighTC() hotstuff.TimeoutCert {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.highTC
}

func (s *ViewStates) NextView() hotstuff.View {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.view++
	return s.view
}

func (s *ViewStates) View() hotstuff.View {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.view
}

// SyncInfo returns the highest known QC or TC.
func (s *ViewStates) SyncInfo() hotstuff.SyncInfo {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return hotstuff.NewSyncInfo().WithQC(s.HighQC()).WithTC(s.HighTC())
}

func (s *ViewStates) UpdateCommittedBlock(block *hotstuff.Block) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.committedBlock = block
}

// CommittedBlock retrieves the last block which was committed.
func (s *ViewStates) CommittedBlock() *hotstuff.Block {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.committedBlock
}
