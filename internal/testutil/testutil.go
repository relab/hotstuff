// Package testutil provides helper methods that are useful for implementing tests.
package testutil

import (
	"crypto/ecdsa"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/internal/mocks"
)

// CreateMockReplica returns a mock of a hotstuff.Replica.
func CreateMockReplica(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key hotstuff.PublicKey) *mocks.MockReplica {
	t.Helper()

	replica := mocks.NewMockReplica(ctrl)
	replica.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)
	replica.
		EXPECT().
		PublicKey().
		AnyTimes().
		Return(key)

	return replica
}

// CreateMockConfig returns a mock of a hotstuff.Config.
func CreateMockConfig(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key hotstuff.PrivateKey) *mocks.MockConfig {
	t.Helper()

	cfg := mocks.NewMockConfig(ctrl)
	cfg.
		EXPECT().
		PrivateKey().
		AnyTimes().
		Return(key)
	cfg.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)

	return cfg
}

// ConfigAddReplica adds a mock replica to a mock configuration.
func ConfigAddReplica(t *testing.T, cfg *mocks.MockConfig, replica *mocks.MockReplica) {
	t.Helper()

	cfg.
		EXPECT().
		Replica(replica.ID()).
		AnyTimes().
		Return(replica, true)
}

// CreateTCPListener creates a net.Listener on a random port.
func CreateTCPListener(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	return lis
}

// Sign creates a signature using the given signer.
func Sign(t *testing.T, hash hotstuff.Hash, signer hotstuff.Signer) hotstuff.Signature {
	t.Helper()
	sig, err := signer.Sign(hash)
	if err != nil {
		t.Fatalf("Failed to sign block: %v", err)
	}
	return sig
}

// CreateSignatures creates partial certificates from multiple signers.
func CreateSignatures(t *testing.T, hash hotstuff.Hash, signers []hotstuff.Signer) []hotstuff.Signature {
	t.Helper()
	sigs := make([]hotstuff.Signature, 0, len(signers))
	for _, signer := range signers {
		sigs = append(sigs, Sign(t, hash, signer))
	}
	return sigs
}

// CreateTimeouts creates a set of TimeoutMsg messages from the given signers.
func CreateTimeouts(t *testing.T, view hotstuff.View, signers []hotstuff.Signer) (timeouts []*hotstuff.TimeoutMsg) {
	t.Helper()
	timeouts = make([]*hotstuff.TimeoutMsg, 0, len(signers))
	sigs := CreateSignatures(t, view.ToHash(), signers)
	for _, sig := range sigs {
		timeouts = append(timeouts, &hotstuff.TimeoutMsg{
			ID:        sig.Signer(),
			View:      view,
			Signature: sig,
		})
	}
	return timeouts
}

// CreatePC creates a partial certificate using the given signer.
func CreatePC(t *testing.T, block *hotstuff.Block, signer hotstuff.Signer) hotstuff.PartialCert {
	t.Helper()
	pc, err := signer.CreatePartialCert(block)
	if err != nil {
		t.Fatalf("Failed to create partial certificate: %v", err)
	}
	return pc
}

// CreatePCs creates one partial certificate using each of the given signers.
func CreatePCs(t *testing.T, block *hotstuff.Block, signers []hotstuff.Signer) []hotstuff.PartialCert {
	t.Helper()
	pcs := make([]hotstuff.PartialCert, 0, len(signers))
	for _, signer := range signers {
		pcs = append(pcs, CreatePC(t, block, signer))
	}
	return pcs
}

// CreateQC creates a QC using the given signers.
func CreateQC(t *testing.T, block *hotstuff.Block, signers []hotstuff.Signer) hotstuff.QuorumCert {
	t.Helper()
	if len(signers) == 0 {
		return nil
	}
	qc, err := signers[0].CreateQuorumCert(block, CreatePCs(t, block, signers))
	if err != nil {
		t.Fatalf("Failed to create QC: %v", err)
	}
	return qc
}

// CreateTC generates a TC using the given signers.
func CreateTC(t *testing.T, view hotstuff.View, signers []hotstuff.Signer) hotstuff.TimeoutCert {
	t.Helper()
	if len(signers) == 0 {
		return nil
	}
	tc, err := signers[0].CreateTimeoutCert(view, CreateTimeouts(t, view, signers))
	if err != nil {
		t.Fatalf("Failed to create TC: %v", err)
	}
	return tc
}

// GenerateKey generates an ECDSA private key for use in tests.
func GenerateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}
