// Package crypto provides implementations of the Crypto interface.
package crypto

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/msg"
)

type crypto struct {
	mods *modules.ConsensusCore
	modules.CryptoBase
}

// New returns a new implementation of the Crypto interface. It will use the given CryptoBase to create and verify
// signatures.
func New(impl modules.CryptoBase) modules.Crypto {
	return &crypto{CryptoBase: impl}
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (c *crypto) InitModule(mods *modules.ConsensusCore, cfg *modules.OptionsBuilder) {
	c.mods = mods
	if mod, ok := c.CryptoBase.(modules.ConsensusModule); ok {
		mod.InitModule(mods, cfg)
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (c crypto) CreatePartialCert(block *msg.Block) (cert msg.PartialCert, err error) {
	sig, err := c.Sign(block.ToBytes())
	if err != nil {
		return msg.PartialCert{}, err
	}
	return *msg.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (c crypto) CreateQuorumCert(block *msg.Block, signatures []msg.PartialCert) (cert msg.QuorumCert, err error) {
	// genesis QC is always valid.
	if block.Hash() == msg.GetGenesis().Hash() {
		return msg.NewQuorumCert(nil, 0, msg.GetGenesis().Hash()), nil
	}
	sigs := make([]msg.QuorumSignature, 0, len(signatures))
	for _, sig := range signatures {
		sigs = append(sigs, sig.Signature())
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return msg.QuorumCert{}, err
	}
	return msg.NewQuorumCert(sig, block.BView(), block.Hash()), nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (c crypto) CreateTimeoutCert(view msg.View, timeouts []msg.TimeoutMsg) (cert msg.TimeoutCert, err error) {
	// view 0 is always valid.
	if view == 0 {
		return msg.NewTimeoutCert(nil, 0), nil
	}
	sigs := make([]msg.QuorumSignature, 0, len(timeouts))
	for _, timeout := range timeouts {
		sigs = append(sigs, timeout.ViewSignature)
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return msg.TimeoutCert{}, err
	}
	return msg.NewTimeoutCert(sig, view), nil
}

// CreateAggregateQC creates an AggregateQC from the given timeout messages.
func (c crypto) CreateAggregateQC(view msg.View, timeouts []msg.TimeoutMsg) (aggQC msg.AggregateQC, err error) {
	qcs := make(map[hotstuff.ID]msg.QuorumCert)
	sigs := make([]msg.QuorumSignature, 0, len(timeouts))
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
		return msg.AggregateQC{}, err
	}
	return msg.NewAggregateQC(qcs, sig, view), nil
}

// VerifyPartialCert verifies a single partial certificate.
func (c crypto) VerifyPartialCert(cert msg.PartialCert) bool {
	block, ok := c.mods.BlockChain().Get(cert.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(cert.Signature(), block.ToBytes())
}

// VerifyQuorumCert verifies a quorum certificate.
func (c crypto) VerifyQuorumCert(qc msg.QuorumCert) bool {
	// genesis QC is always valid.
	if qc.BlockHash() == msg.GetGenesis().Hash() {
		return true
	}
	if qc.Signature().Participants().Len() < c.mods.Configuration().QuorumSize() {
		return false
	}
	block, ok := c.mods.BlockChain().Get(qc.BlockHash())
	if !ok {
		return false
	}
	return c.Verify(qc.Signature(), block.ToBytes())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (c crypto) VerifyTimeoutCert(tc msg.TimeoutCert) bool {
	// view 0 TC is always valid.
	if tc.TCView() == 0 {
		return true
	}
	if tc.Signature().Participants().Len() < c.mods.Configuration().QuorumSize() {
		return false
	}
	return c.Verify(tc.Signature(), tc.TCView().ToBytes())
}

// VerifyAggregateQC verifies the AggregateQC and returns the highQC, if valid.
func (c crypto) VerifyAggregateQC(aggQC msg.AggregateQC) (highQC msg.QuorumCert, ok bool) {
	messages := make(map[hotstuff.ID][]byte)
	for id, qc := range aggQC.QCerts() {
		if highQC.QCView() < qc.QCView() || highQC == (msg.QuorumCert{}) {
			highQC = qc
		}
		// reconstruct the TimeoutMsg to get the hash
		messages[id] = msg.NewTimeoutMsg(id, aggQC.AQCView(), msg.NewSyncInfo().WithQC(qc), nil).ToBytes()
	}
	if aggQC.Signature().Participants().Len() < c.mods.Configuration().QuorumSize() {
		return msg.QuorumCert{}, false
	}
	// both the batched aggQC signatures and the highQC must be verified
	if c.BatchVerify(aggQC.Signature(), messages) && c.VerifyQuorumCert(highQC) {
		return highQC, true
	}
	return msg.QuorumCert{}, false
}
