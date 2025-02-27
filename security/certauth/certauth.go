// Package crypto provides implementations of the Crypto interface.
package certauth

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
)

type CertAuthority struct {
	modules.CryptoBase // embedded to avoid having to implement forwarding methods

	logger     logging.Logger
	blockChain *blockchain.BlockChain
}

// New returns a CertAuthority. It will use the given CryptoBase to create and verify
// signatures.
func New(
	impl modules.CryptoBase,

	blockChain *blockchain.BlockChain,
	logger logging.Logger,
) *CertAuthority {
	return &CertAuthority{
		CryptoBase: impl,

		logger:     logger,
		blockChain: blockChain,
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (c CertAuthority) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := c.Sign(block.ToBytes())
	if err != nil {
		return hotstuff.PartialCert{}, err
	}
	return hotstuff.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (c CertAuthority) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
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
func (c CertAuthority) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
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
func (c CertAuthority) CreateAggregateQC(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (aggQC hotstuff.AggregateQC, err error) {
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
func (c CertAuthority) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	block, ok := c.blockChain.Get(cert.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(cert.Signature(), block.ToBytes())
}

// VerifyQuorumCert verifies a quorum certificate.
func (c CertAuthority) VerifyQuorumCert(quorumSize int, qc hotstuff.QuorumCert) bool {
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
func (c CertAuthority) VerifyTimeoutCert(quorumSize int, tc hotstuff.TimeoutCert) bool {
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
func (c CertAuthority) VerifyAggregateQC(quorumSize int, aggQC hotstuff.AggregateQC) (highQC hotstuff.QuorumCert, ok bool) {
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
