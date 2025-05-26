package viewstates

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type ViewStates struct {
	logger     logging.Logger
	blockChain *blockchain.BlockChain
	auth       *certauth.CertAuthority

	mut sync.RWMutex // to protect the following

	highTC hotstuff.TimeoutCert
	highQC hotstuff.QuorumCert
	view   hotstuff.View
}

func New(
	logger logging.Logger,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
) *ViewStates {
	s := &ViewStates{
		logger:     logger,
		blockChain: blockChain,
		auth:       auth,

		view: 1,
	}
	var err error
	s.highQC, err = s.auth.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.auth.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
	return s
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
// TODO(AlanRostem): this was in synchronizer, make tests.
func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) {
	newBlock, ok := s.blockChain.Get(qc.BlockHash())
	if !ok {
		s.logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}
	if newBlock.View() > s.highQC.View() {
		s.highQC = qc
		s.logger.Debug("HighQC updated")
	}
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
// TODO(AlanRostem): this was in synchronizer, make tests.
func (s *ViewStates) UpdateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.logger.Debug("HighTC updated")
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

func (s *ViewStates) UpdateView(v hotstuff.View) {
	s.view = v
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
