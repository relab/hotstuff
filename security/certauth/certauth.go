// Package crypto provides implementations of the Crypto interface.
package certauth

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
)

// TODO(AlanRostem): propose better name: CertMgr
type CertAuthority struct {
	modules.CryptoBase // embedded to avoid having to implement forwarding methods
	config             *core.RuntimeConfig
	logger             logging.Logger
	blockChain         *blockchain.BlockChain

	highTC hotstuff.TimeoutCert
	highQC hotstuff.QuorumCert
}

// New returns a CertAuthority. It will use the given CryptoBase to create and verify
// signatures.
func New(
	config *core.RuntimeConfig,
	logger logging.Logger,
	blockChain *blockchain.BlockChain,
	impl modules.CryptoBase,
	opts ...Option,
) *CertAuthority {
	ca := &CertAuthority{
		CryptoBase: impl,
		config:     config,
		logger:     logger,
		blockChain: blockChain,
	}
	for _, opt := range opts {
		opt(ca)
	}

	var err error
	ca.highQC, err = ca.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	ca.highTC, err = ca.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
	return ca
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (c *CertAuthority) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := c.Sign(block.ToBytes())
	if err != nil {
		return hotstuff.PartialCert{}, err
	}
	return hotstuff.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (c *CertAuthority) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	// genesis QC is always valid.
	if block.Hash() == hotstuff.GetGenesis().Hash() {
		return hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), nil
	}
	sigs := make([]hotstuff.QuorumSignature, 0, len(signatures))
	for _, sig := range signatures {
		sigs = append(sigs, sig.Signature())
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return hotstuff.QuorumCert{}, err
	}
	return hotstuff.NewQuorumCert(sig, block.View(), block.Hash()), nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (c *CertAuthority) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
	// view 0 is always valid.
	if view == 0 {
		return hotstuff.NewTimeoutCert(nil, 0), nil
	}
	sigs := make([]hotstuff.QuorumSignature, 0, len(timeouts))
	for _, timeout := range timeouts {
		sigs = append(sigs, timeout.ViewSignature)
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return hotstuff.TimeoutCert{}, err
	}
	return hotstuff.NewTimeoutCert(sig, view), nil
}

// CreateAggregateQC creates an AggregateQC from the given timeout messages.
func (c *CertAuthority) CreateAggregateQC(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (aggQC hotstuff.AggregateQC, err error) {
	qcs := make(map[hotstuff.ID]hotstuff.QuorumCert)
	sigs := make([]hotstuff.QuorumSignature, 0, len(timeouts))
	for _, timeout := range timeouts {
		if qc, ok := timeout.SyncInfo.QC(); ok {
			qcs[timeout.ID] = qc
		}
		if timeout.MsgSignature != nil {
			sigs = append(sigs, timeout.MsgSignature)
		}
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return hotstuff.AggregateQC{}, err
	}
	return hotstuff.NewAggregateQC(qcs, sig, view), nil
}

// VerifyPartialCert verifies a single partial certificate.
func (c *CertAuthority) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	block, ok := c.blockChain.Get(cert.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(cert.Signature(), block.ToBytes())
}

// VerifyQuorumCert verifies a quorum certificate.
func (c *CertAuthority) VerifyQuorumCert(quorumSize int, qc hotstuff.QuorumCert) bool {
	// genesis QC is always valid.
	if qc.BlockHash() == hotstuff.GetGenesis().Hash() {
		return true
	}

	// TODO: FIX BUG - qcSignature can be nil when a leader is byzantine.
	qcSignature := qc.Signature()
	if qcSignature == nil {
		c.logger.DPanicf("quorum certificate has nil signature (view=%d)", qc.View())
	}

	participants := qcSignature.Participants()
	if participants.Len() < quorumSize {
		return false
	}
	block, ok := c.blockChain.Get(qc.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(qc.Signature(), block.ToBytes())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (c *CertAuthority) VerifyTimeoutCert(quorumSize int, tc hotstuff.TimeoutCert) bool {
	// view 0 TC is always valid.
	if tc.View() == 0 {
		return true
	}
	if tc.Signature().Participants().Len() < quorumSize {
		return false
	}
	return c.Verify(tc.Signature(), tc.View().ToBytes())
}

// VerifyAggregateQC verifies the AggregateQC and returns the highQC, if valid.
func (c *CertAuthority) VerifyAggregateQC(quorumSize int, aggQC hotstuff.AggregateQC) (highQC hotstuff.QuorumCert, ok bool) {
	messages := make(map[hotstuff.ID][]byte)
	for id, qc := range aggQC.QCs() {
		if highQC.View() < qc.View() || highQC == (hotstuff.QuorumCert{}) {
			highQC = qc
		}
		// reconstruct the TimeoutMsg to get the hash
		messages[id] = hotstuff.TimeoutMsg{
			ID:       id,
			View:     aggQC.View(),
			SyncInfo: hotstuff.NewSyncInfo().WithQC(qc),
		}.ToBytes()
	}
	if aggQC.Sig().Participants().Len() < quorumSize {
		return hotstuff.QuorumCert{}, false
	}
	// both the batched aggQC signatures and the highQC must be verified
	if c.BatchVerify(aggQC.Sig(), messages) && c.VerifyQuorumCert(quorumSize, highQC) {
		return highQC, true
	}
	return hotstuff.QuorumCert{}, false
}

// VerifyProposal is a helper that verifies the QC of the block in the proposal message.
// TODO(AlanRostem): add a test case for this method.
func (c *CertAuthority) VerifyProposal(proposal *hotstuff.ProposeMsg) bool {
	block := proposal.Block
	view := block.View()
	if c.config.HasAggregateQC() && proposal.AggregateQC != nil {
		highQC, ok := c.VerifyAggregateQC(c.config.QuorumSize(), *proposal.AggregateQC)
		if !ok {
			c.logger.Warnf("VerifyProposal[view=%d]: failed to verify aggregate QC", view)
			return false
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			c.logger.Warnf("VerifyProposal[view=%d]: block QC does not equal highQC", view)
			return false
		}
	}
	if !c.VerifyQuorumCert(c.config.QuorumSize(), block.QuorumCert()) {
		c.logger.Infof("VerifyProposal[view=%d]: invalid QC", view)
		return false
	}
	return true
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
// TODO(AlanRostem): this was in synchronizer, make tests.
func (s *CertAuthority) UpdateHighQC(qc hotstuff.QuorumCert) {
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
func (s *CertAuthority) UpdateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.logger.Debug("HighTC updated")
	}
}

func (s *CertAuthority) HighQC() hotstuff.QuorumCert {
	return s.highQC
}

func (s *CertAuthority) HighTC() hotstuff.TimeoutCert {
	return s.highTC
}
