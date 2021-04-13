package crypto

import "github.com/relab/hotstuff"

type base struct {
	hotstuff.CryptoImpl
}

// New returns a new base implementation of the Crypto interface. It will use the given CryptoImpl to create and verify
// signatures.
func New(impl hotstuff.CryptoImpl) hotstuff.Crypto {
	return base{CryptoImpl: impl}
}

// InitModule gives the module a reference to the HotStuff object.
func (base base) InitModule(hs *hotstuff.HotStuff, cfg *hotstuff.ConfigBuilder) {
	if mod, ok := base.CryptoImpl.(hotstuff.Module); ok {
		mod.InitModule(hs, cfg)
	}
}

// CreatePartialCert signs a single block and returns the partial certificate.
func (base base) CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error) {
	sig, err := base.Sign(block.Hash())
	if err != nil {
		return hotstuff.PartialCert{}, err
	}
	return hotstuff.NewPartialCert(sig, block.Hash()), nil
}

// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
func (base base) CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error) {
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
func (base base) CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error) {
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
func (base base) VerifyPartialCert(cert hotstuff.PartialCert) bool {
	return base.Verify(cert.Signature(), cert.BlockHash())
}

// VerifyQuorumCert verifies a quorum certificate.
func (base base) VerifyQuorumCert(qc hotstuff.QuorumCert) bool {
	if qc.BlockHash() == hotstuff.GetGenesis().Hash() {
		return true
	}
	return base.VerifyThresholdSignature(qc.Signature(), qc.BlockHash())
}

// VerifyTimeoutCert verifies a timeout certificate.
func (base base) VerifyTimeoutCert(tc hotstuff.TimeoutCert) bool {
	if tc.View() == 0 {
		return true
	}
	return base.VerifyThresholdSignature(tc.Signature(), tc.View().ToHash())
}
