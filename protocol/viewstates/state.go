package viewstates

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type States struct {
	logger     logging.Logger
	blockChain *blockchain.BlockChain
	auth       *certauth.CertAuthority

	highTC hotstuff.TimeoutCert
	highQC hotstuff.QuorumCert
}

func New(
	logger logging.Logger,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
) *States {
	s := &States{
		logger:     logger,
		blockChain: blockChain,
		auth:       auth,
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
func (s *States) UpdateHighQC(qc hotstuff.QuorumCert) {
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
func (s *States) UpdateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.logger.Debug("HighTC updated")
	}
}

func (s *States) HighQC() hotstuff.QuorumCert {
	return s.highQC
}

func (s *States) HighTC() hotstuff.TimeoutCert {
	return s.highTC
}
