// Package protocol maintains the protocol's view states, which include
// the high quorum certificate (HighQC), high timeout certificate (HighTC),
// the current view, and the last committed block.
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

	mut            sync.RWMutex // protects the following fields:
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

// UpdateHighQC updates HighQC if quorum certificate's block is for a higher view
// and the block extends the committed chain.
// It returns true if HighQC was updated. It returns an error if:
// - the quorum certificate's block is not found in the local blockchain
// - the block does not extend the committed chain (safety check)
func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) (bool, error) {
	newBlock, ok := s.blockchain.Get(qc.BlockHash())
	if !ok {
		return false, fmt.Errorf("block %x not found for QC@view %d", qc.BlockHash(), qc.View())
	}

	// Get committed block reference before acquiring lock
	// to avoid potential deadlock with blockchain operations
	s.mut.RLock()
	committedBlock := s.committedBlock
	currentHighQCView := s.highQC.View()
	s.mut.RUnlock()

	// Check if new block has higher view than current HighQC
	if newBlock.View() <= currentHighQCView {
		return false, nil
	}

	// Safety check: new QC must extend the committed chain
	// This prevents HighQC from being updated to a fork that doesn't
	// include the already committed blocks
	if committedBlock != nil && committedBlock.View() > 0 {
		if !s.blockchain.Extends(newBlock, committedBlock) {
			return false, fmt.Errorf(
				"QC block (view=%d, hash=%.8x) does not extend committed chain (view=%d, hash=%.8x)",
				newBlock.View(), qc.BlockHash(), committedBlock.View(), committedBlock.Hash())
		}
	}

	// All checks passed, update HighQC
	s.mut.Lock()
	defer s.mut.Unlock()
	// Double-check view in case of concurrent updates
	if newBlock.View() <= s.highQC.View() {
		return false, nil
	}
	s.highQC = qc
	return true, nil
}

// UpdateHighTC updates HighTC if timeout certificate's view is higher than the current HighTC.
func (s *ViewStates) UpdateHighTC(tc hotstuff.TimeoutCert) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if tc.View() > s.highTC.View() {
		s.highTC = tc
	}
}

// HighQC returns the highest known quorum certificate.
func (s *ViewStates) HighQC() hotstuff.QuorumCert {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.highQC
}

// HighTC returns the highest known timeout certificate.
func (s *ViewStates) HighTC() hotstuff.TimeoutCert {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.highTC
}

// NextView increments the current view and returns the new view.
func (s *ViewStates) NextView() hotstuff.View {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.view++
	return s.view
}

// View returns the current view.
func (s *ViewStates) View() hotstuff.View {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.view
}

// SyncInfo returns the highest known QC or TC.
func (s *ViewStates) SyncInfo() hotstuff.SyncInfo {
	si := hotstuff.NewSyncInfo()
	s.mut.RLock()
	si.SetQC(s.highQC)
	si.SetTC(s.highTC)
	s.mut.RUnlock()
	return si
}

// UpdateCommittedBlock updates the last committed block.
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

// FetchBlock attempts to fetch a block by its hash.
// If the block is not available locally, it will try to fetch it from the network.
// This is used to ensure blocks referenced by QCs are available before verification.
func (s *ViewStates) FetchBlock(hash hotstuff.Hash) {
	s.blockchain.Get(hash)
}
