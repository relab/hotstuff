// Package crypto provides implementations of the Crypto interface.
package crypto

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/msg"
)

type base struct {
	consensus.CryptoImpl
}

// New returns a new base implementation of the Crypto interface. It will use the given CryptoImpl to create and verify
// signatures.
func New(impl consensus.CryptoImpl) consensus.Crypto {
	return base{CryptoImpl: impl}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (base base) InitConsensusModule(mods *consensus.Modules, cfg *consensus.OptionsBuilder) {
	if mod, ok := base.CryptoImpl.(consensus.Module); ok {
		mod.InitConsensusModule(mods, cfg)
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (base base) CreatePartialCert(block *msg.Block) (cert msg.PartialCert, err error) {
	sig, err := base.Sign(block.Hash())
	if err != nil {
		return msg.PartialCert{}, err
	}
	return msg.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (base base) CreateQuorumCert(block *msg.Block, signatures []msg.PartialCert) (cert msg.QuorumCert, err error) {
	// genesis QC is always valid.
	if block.Hash() == msg.GetGenesis().Hash() {
		return msg.NewQuorumCert(nil, 0, msg.GetGenesis().Hash()), nil
	}
	sigs := make([]msg.Signature, 0, len(signatures))
	for _, sig := range signatures {
		sigs = append(sigs, sig.Signature())
	}
	sig, err := base.CreateThresholdSignature(sigs, block.Hash())
	if err != nil {
		return msg.QuorumCert{}, err
	}
	return msg.NewQuorumCert(sig, block.View(), block.Hash()), nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (base base) CreateTimeoutCert(view msg.View, timeouts []msg.TimeoutMsg) (cert msg.TimeoutCert, err error) {
	// view 0 is always valid.
	if view == 0 {
		return msg.NewTimeoutCert(nil, 0), nil
	}
	sigs := make([]msg.Signature, 0, len(timeouts))
	for _, timeout := range timeouts {
		sigs = append(sigs, timeout.ViewSignature)
	}
	sig, err := base.CreateThresholdSignature(sigs, view.ToHash())
	if err != nil {
		return msg.TimeoutCert{}, err
	}
	return msg.NewTimeoutCert(sig, view), nil
}

func (base base) CreateAggregateQC(view msg.View, timeouts []msg.TimeoutMsg) (aggQC msg.AggregateQC, err error) {
	qcs := make(map[hotstuff.ID]msg.QuorumCert)
	sigs := make([]msg.Signature, 0, len(timeouts))
	hashes := make(map[hotstuff.ID]msg.Hash)
	for _, timeout := range timeouts {
		if qc, ok := timeout.SyncInfo.QC(); ok {
			qcs[timeout.ID] = qc
		}
		if timeout.MsgSignature != nil {
			sigs = append(sigs, timeout.MsgSignature)
			hashes[timeout.ID] = timeout.Hash()
		}
	}
	sig, err := base.CreateThresholdSignatureForMessageSet(sigs, hashes)
	if err != nil {
		return aggQC, err
	}
	return msg.NewAggregateQC(qcs, sig, view), nil
}

// VerifyPartialCert verifies a single partial certificate.
func (base base) VerifyPartialCert(cert msg.PartialCert) bool {
	return base.Verify(cert.Signature(), cert.BlockHash())
}

// VerifyQuorumCert verifies a quorum certificate.
func (base base) VerifyQuorumCert(qc msg.QuorumCert) bool {
	if qc.BlockHash() == msg.GetGenesis().Hash() {
		return true
	}
	return base.VerifyThresholdSignature(qc.Signature(), qc.BlockHash())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (base base) VerifyTimeoutCert(tc msg.TimeoutCert) bool {
	if tc.View() == 0 {
		return true
	}
	return base.VerifyThresholdSignature(tc.Signature(), tc.View().ToHash())
}

// VerifyAggregateQC verifies the AggregateQC and returns the highQC, if valid.
func (base base) VerifyAggregateQC(aggQC msg.AggregateQC) (bool, msg.QuorumCert) {
	var highQC *msg.QuorumCert
	hashes := make(map[hotstuff.ID]msg.Hash)
	for id, qc := range aggQC.QCs() {
		if highQC == nil {
			highQC = new(msg.QuorumCert)
			*highQC = qc
		} else if highQC.View() < qc.View() {
			*highQC = qc
		}

		// reconstruct the TimeoutMsg to get the hash
		hashes[id] = msg.TimeoutMsg{
			ID:       id,
			View:     aggQC.View(),
			SyncInfo: msg.NewSyncInfo().WithQC(qc),
		}.Hash()
	}
	ok := base.VerifyThresholdSignatureForMessageSet(aggQC.Sig(), hashes)
	if !ok {
		return false, msg.QuorumCert{}
	}
	if base.VerifyQuorumCert(*highQC) {
		return true, *highQC
	}
	return false, msg.QuorumCert{}
}
