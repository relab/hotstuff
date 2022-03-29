// Package crypto provides implementations of the Crypto interface.
package crypto

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type crypto struct {
	mods *consensus.Modules
	consensus.CryptoBase
}

// New returns a new implementation of the Crypto interface. It will use the given CryptoBase to create and verify
// signatures.
func New(impl consensus.CryptoBase) consensus.Crypto {
	return &crypto{CryptoBase: impl}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (c *crypto) InitConsensusModule(mods *consensus.Modules, cfg *consensus.OptionsBuilder) {
	c.mods = mods
	if mod, ok := c.CryptoBase.(consensus.Module); ok {
		mod.InitConsensusModule(mods, cfg)
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (c crypto) CreatePartialCert(block *consensus.Block) (cert consensus.PartialCert, err error) {
	sig, err := c.Sign(block.ToBytes())
	if err != nil {
		return consensus.PartialCert{}, err
	}
	return consensus.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (c crypto) CreateQuorumCert(block *consensus.Block, signatures []consensus.PartialCert) (cert consensus.QuorumCert, err error) {
	// genesis QC is always valid.
	if block.Hash() == consensus.GetGenesis().Hash() {
		return consensus.NewQuorumCert(nil, 0, consensus.GetGenesis().Hash()), nil
	}
	sigs := make([]consensus.QuorumSignature, 0, len(signatures))
	for _, sig := range signatures {
		sigs = append(sigs, sig.Signature())
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return consensus.QuorumCert{}, err
	}
	return consensus.NewQuorumCert(sig, block.View(), block.Hash()), nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (c crypto) CreateTimeoutCert(view consensus.View, timeouts []consensus.TimeoutMsg) (cert consensus.TimeoutCert, err error) {
	// view 0 is always valid.
	if view == 0 {
		return consensus.NewTimeoutCert(nil, 0), nil
	}
	sigs := make([]consensus.QuorumSignature, 0, len(timeouts))
	for _, timeout := range timeouts {
		sigs = append(sigs, timeout.ViewSignature)
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return consensus.TimeoutCert{}, err
	}
	return consensus.NewTimeoutCert(sig, view), nil
}

func (c crypto) CreateAggregateQC(view consensus.View, timeouts []consensus.TimeoutMsg) (aggQC consensus.AggregateQC, err error) {
	qcs := make(map[hotstuff.ID]consensus.QuorumCert)
	sigs := make([]consensus.QuorumSignature, 0, len(timeouts))
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
		return consensus.AggregateQC{}, err
	}
	return consensus.NewAggregateQC(qcs, sig, view), nil
}

// VerifyPartialCert verifies a single partial certificate.
func (c crypto) VerifyPartialCert(cert consensus.PartialCert) bool {
	block, ok := c.mods.BlockChain().Get(cert.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(cert.Signature(), consensus.VerifySingle(block.ToBytes()))
}

// VerifyQuorumCert verifies a quorum certificate.
func (c crypto) VerifyQuorumCert(qc consensus.QuorumCert) bool {
	// genesis QC is always valid.
	if qc.BlockHash() == consensus.GetGenesis().Hash() {
		return true
	}
	block, ok := c.mods.BlockChain().Get(qc.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(qc.Signature(), consensus.VerifySingle(block.ToBytes()))
}

// VerifyTimeoutCert verifies a timeout certificate.
func (c crypto) VerifyTimeoutCert(tc consensus.TimeoutCert) bool {
	// view 0 TC is always valid.
	if tc.View() == 0 {
		return true
	}
	return c.Verify(tc.Signature(), consensus.VerifySingle(tc.View().ToBytes()))
}

// VerifyAggregateQC verifies the AggregateQC and returns the highQC, if valid.
func (c crypto) VerifyAggregateQC(aggQC consensus.AggregateQC) (bool, consensus.QuorumCert) {
	var highQC *consensus.QuorumCert
	messages := make(map[hotstuff.ID][]byte)
	for id, qc := range aggQC.QCs() {
		if highQC == nil {
			highQC = new(consensus.QuorumCert)
			*highQC = qc
		} else if highQC.View() < qc.View() {
			*highQC = qc
		}

		// reconstruct the TimeoutMsg to get the hash
		messages[id] = consensus.TimeoutMsg{
			ID:       id,
			View:     aggQC.View(),
			SyncInfo: consensus.NewSyncInfo().WithQC(qc),
		}.ToBytes()
	}
	ok := c.Verify(aggQC.Sig(), consensus.VerifyMulti(messages))
	if !ok {
		return false, consensus.QuorumCert{}
	}
	if c.VerifyQuorumCert(*highQC) {
		return true, *highQC
	}
	return false, consensus.QuorumCert{}
}
