// Package cert provides a certificate authority for creating and verifying quorum certificates.
package cert

import (
	"fmt"
	"sort"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/crypto"
)

type Authority struct {
	crypto.Base // embedded to avoid having to implement forwarding methods
	config      *core.RuntimeConfig
	blockchain  *blockchain.Blockchain
}

// NewAuthority returns an Authority. It will use the given CryptoBase to create and verify
// signatures.
func NewAuthority(
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	base crypto.Base,
	opts ...Option,
) *Authority {
	ca := &Authority{
		Base:       base,
		config:     config,
		blockchain: blockchain,
	}
	for _, opt := range opts {
		opt(ca)
	}
	return ca
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (c *Authority) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := c.Sign(block.ToBytes())
	if err != nil {
		return hotstuff.PartialCert{}, err
	}
	return hotstuff.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (c *Authority) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
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
func (c *Authority) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
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
func (c *Authority) CreateAggregateQC(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (aggQC hotstuff.AggregateQC, err error) {
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
func (c *Authority) VerifyPartialCert(cert hotstuff.PartialCert) error {
	block, ok := c.blockchain.Get(cert.BlockHash())
	if !ok {
		return fmt.Errorf("block not found: %v", cert.BlockHash())
	}
	return c.Verify(cert.Signature(), block.ToBytes())
}

// VerifyQuorumCert verifies a quorum certificate.
func (c *Authority) VerifyQuorumCert(qc hotstuff.QuorumCert) error {
	// genesis QC is always valid.
	if qc.BlockHash() == hotstuff.GetGenesis().Hash() {
		return nil
	}

	// TODO: FIX BUG - qcSignature can be nil when a leader is byzantine.
	qcSignature := qc.Signature()
	if qcSignature == nil {
		return fmt.Errorf("quorum certificate has nil signature (view=%d)", qc.View())
	}

	participants := qcSignature.Participants()
	quorumSize := c.config.QuorumSize()
	if participants.Len() < quorumSize {
		return fmt.Errorf("%d participants cannot satisfy the quorum requirement: %d", participants.Len(), quorumSize)
	}
	block, ok := c.blockchain.Get(qc.BlockHash())
	if !ok {
		return fmt.Errorf("block not found: %v", qc.BlockHash())
	}
	return c.Verify(qc.Signature(), block.ToBytes())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (c *Authority) VerifyTimeoutCert(tc hotstuff.TimeoutCert) error {
	// view 0 TC is always valid.
	if tc.View() == 0 {
		return nil
	}
	quorumSize := c.config.QuorumSize()
	participants := tc.Signature().Participants()
	if participants.Len() < quorumSize {
		return fmt.Errorf("%d participants cannot satisfy the quorum requirement: %d", participants.Len(), quorumSize)
	}
	return c.Verify(tc.Signature(), tc.View().ToBytes())
}

// VerifyAggregateQC verifies the AggregateQC and returns the highQC, if valid.
func (c *Authority) VerifyAggregateQC(aggQC hotstuff.AggregateQC) (highQC hotstuff.QuorumCert, err error) {
	messages := make(map[hotstuff.ID][]byte)
	qcs := make(map[hotstuff.View][]hotstuff.QuorumCert, len(aggQC.QCs()))
	for id, qc := range aggQC.QCs() {
		qcs[qc.View()] = append(qcs[qc.View()], qc)
		// reconstruct the TimeoutMsg to get the hash
		messages[id] = hotstuff.TimeoutMsg{
			ID:       id,
			View:     aggQC.View(),
			SyncInfo: hotstuff.NewSyncInfo().WithQC(qc),
		}.ToBytes()
	}
	quorumSize := c.config.QuorumSize()
	participants := aggQC.Sig().Participants()
	if participants.Len() < quorumSize {
		return hotstuff.QuorumCert{}, fmt.Errorf("%d participants cannot satisfy the quorum requirement: %d", participants.Len(), quorumSize)
	}
	// both the batched aggQC signatures and the highQC must be verified
	if err := c.BatchVerify(aggQC.Sig(), messages); err != nil {
		return hotstuff.QuorumCert{}, err
	}
	// After verifying the aggregate signature, find the highest-view valid QC from the set.
	// This individual QC will serve as the highQC for the protocol.
	highQC, err = c.findHighestValidQC(qcs)
	if err != nil {
		return hotstuff.QuorumCert{}, err
	}
	return highQC, nil
}

func (c *Authority) findHighestValidQC(qcs map[hotstuff.View][]hotstuff.QuorumCert) (highQC hotstuff.QuorumCert, err error) {
	views := make([]hotstuff.View, 0, len(qcs))
	for v := range qcs {
		views = append(views, v)
	}
	// Sort views in descending order to check the highest view first.
	sort.Slice(views, func(i, j int) bool {
		return views[i] > views[j]
	})
	for _, view := range views {
		for _, qc := range qcs[view] {
			if err := c.VerifyQuorumCert(qc); err == nil {
				return qc, nil
			}
		}
	}
	// If the loop completes without finding any valid QC, return an error.
	return hotstuff.QuorumCert{}, fmt.Errorf("no valid high quorum certificate found in aggregate QC")
}

// VerifyAnyQC is a helper that verifies either a QC or the aggregateQC in a proposal message.
func (c *Authority) VerifyAnyQC(proposal *hotstuff.ProposeMsg) error {
	qc := proposal.Block.QuorumCert()
	aggQC := proposal.AggregateQC
	if c.config.HasAggregateQC() && aggQC != nil {
		highQC, err := c.VerifyAggregateQC(*aggQC)
		if err != nil {
			return err
		}
		// for simplicity, we require that the highQC found in the AggregateQC equals the block's QC.
		if !qc.Equals(highQC) {
			return fmt.Errorf("block QC does not match the highQC of the block's aggregate QC")
		}
	}
	return c.VerifyQuorumCert(qc)
}
