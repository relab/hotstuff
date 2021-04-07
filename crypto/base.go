package crypto

import "github.com/relab/hotstuff"

type Base struct {
	hotstuff.CryptoImpl
}

func New(impl hotstuff.CryptoImpl) Base {
	return Base{CryptoImpl: impl}
}

// InitModule gives the module a reference to the HotStuff object.
func (base Base) InitModule(hs *hotstuff.HotStuff) {
	if mod, ok := base.CryptoImpl.(hotstuff.Module); ok {
		mod.InitModule(hs)
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (base Base) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := base.Sign(block.Hash())
	if err != nil {
		return hotstuff.PartialCert{}, err
	}
	return hotstuff.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (base Base) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
	// genesis QC is always valid.
	if block.Hash() == hotstuff.GetGenesis().Hash() {
		return hotstuff.NewQuorumCert(nil, hotstuff.GetGenesis().Hash()), nil
	}
	sigs := make([]hotstuff.Signature, 0, len(signatures))
	for _, sig := range signatures {
		sigs = append(sigs, sig.Signature())
	}
	sig, err := base.CreateThresholdSignature(sigs, block.Hash())
	if err != nil {
		return hotstuff.QuorumCert{}, err
	}
	return hotstuff.NewQuorumCert(sig, block.Hash()), nil
}

// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
func (base Base) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
	// view 0 is always valid.
	if view == 0 {
		return hotstuff.NewTimeoutCert(nil, 0), nil
	}
	sigs := make([]hotstuff.Signature, 0, len(timeouts))
	for _, timeout := range timeouts {
		sigs = append(sigs, timeout.Signature)
	}
	sig, err := base.CreateThresholdSignature(sigs, view.ToHash())
	if err != nil {
		return hotstuff.TimeoutCert{}, err
	}
	return hotstuff.NewTimeoutCert(sig, view), nil
}

// VerifyPartialCert verifies a single partial certificate.
func (base Base) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	return base.Verify(cert.Signature(), cert.BlockHash())
}

// VerifyQuorumCert verifies a quorum certificate.
func (base Base) VerifyQuorumCert(qc hotstuff.QuorumCert) bool {
	if qc.BlockHash() == hotstuff.GetGenesis().Hash() {
		return true
	}
	return base.VerifyThresholdSignature(qc.Signature(), qc.BlockHash())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (base Base) VerifyTimeoutCert(tc hotstuff.TimeoutCert) bool {
	if tc.View() == 0 {
		return true
	}
	return base.VerifyThresholdSignature(tc.Signature(), tc.View().ToHash())
}
