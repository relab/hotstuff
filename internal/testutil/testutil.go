// Package testutil provides helper methods that are useful for implementing tests.
package testutil

import (
	"net"
	"testing"

	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/security/cert"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/keygen"
)

// TODO(AlanRostem): create a test for server.go using this in another PR.
// CreateTCPListener creates a net.Listener on a random port.
func CreateTCPListener(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	return lis
}

func CreateBlock(t *testing.T, signer *cert.Authority) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, &clientpb.Batch{
		Commands: []*clientpb.Command{},
	}, 42, 1)
	return b
}

func CreateValidBlock(t *testing.T, proposer hotstuff.ID, validParent *hotstuff.Block) *hotstuff.Block {
	t.Helper()
	// TODO(AlanRostem): consider creating a qc with a valid signature too
	qc := hotstuff.NewQuorumCert(nil, validParent.View(), validParent.Hash())
	return hotstuff.NewBlock(
		validParent.Hash(),
		qc,
		&clientpb.Batch{},
		validParent.View()+1,
		proposer,
	)
}

// CreateSignatures creates partial certificates from multiple signers.
func CreateSignatures(t *testing.T, message []byte, signers []*cert.Authority) []hotstuff.QuorumSignature {
	t.Helper()
	sigs := make([]hotstuff.QuorumSignature, 0, len(signers))
	for _, signer := range signers {
		sig, err := signer.Sign(message)
		if err != nil {
			t.Fatalf("Failed to sign block: %v", err)
		}
		sigs = append(sigs, sig)
	}
	return sigs
}

func signer(s hotstuff.QuorumSignature) hotstuff.ID {
	var signer hotstuff.ID
	s.Participants().RangeWhile(func(i hotstuff.ID) bool {
		signer = i
		return false
	})
	return signer
}

// CreateTimeouts creates a set of TimeoutMsg messages from the given signers.
func CreateTimeouts(t *testing.T, view hotstuff.View, signers []*cert.Authority) (timeouts []hotstuff.TimeoutMsg) {
	t.Helper()
	timeouts = make([]hotstuff.TimeoutMsg, 0, len(signers))
	viewSigs := CreateSignatures(t, view.ToBytes(), signers)
	for _, sig := range viewSigs {
		timeouts = append(timeouts, hotstuff.TimeoutMsg{
			ID:            signer(sig),
			View:          view,
			ViewSignature: sig,
			SyncInfo:      hotstuff.NewSyncInfo().WithQC(hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())),
		})
	}
	for i := range timeouts {
		sig, err := signers[i].Sign(timeouts[i].ToBytes())
		if err != nil {
			t.Fatalf("Failed to sign timeout message: %v", err)
		}
		timeouts[i].MsgSignature = sig
	}
	return timeouts
}

// CreatePC creates a partial certificate using the given signer.
func CreatePC(t *testing.T, block *hotstuff.Block, signer *cert.Authority) hotstuff.PartialCert {
	t.Helper()
	pc, err := signer.CreatePartialCert(block)
	if err != nil {
		t.Fatalf("Failed to create partial certificate: %v", err)
	}
	return pc
}

// CreatePCs creates one partial certificate using each of the given signers.
func CreatePCs(t *testing.T, block *hotstuff.Block, signers []*cert.Authority) []hotstuff.PartialCert {
	t.Helper()
	pcs := make([]hotstuff.PartialCert, 0, len(signers))
	for _, signer := range signers {
		pcs = append(pcs, CreatePC(t, block, signer))
	}
	return pcs
}

// CreateQC creates a QC using the given signers.
func CreateQC(t *testing.T, block *hotstuff.Block, signers []*cert.Authority) hotstuff.QuorumCert {
	t.Helper()
	if len(signers) == 0 {
		return hotstuff.QuorumCert{}
	}
	qc, err := signers[0].CreateQuorumCert(block, CreatePCs(t, block, signers))
	if err != nil {
		t.Fatalf("Failed to create QC: %v", err)
	}
	return qc
}

// CreateTC generates a TC using the given signers.
func CreateTC(t *testing.T, view hotstuff.View, timeoutCreator *cert.Authority, otherSigners []*cert.Authority) hotstuff.TimeoutCert {
	t.Helper()
	if timeoutCreator == nil || len(otherSigners) == 0 {
		return hotstuff.TimeoutCert{}
	}
	tc, err := timeoutCreator.CreateTimeoutCert(view, CreateTimeouts(t, view, otherSigners))
	if err != nil {
		t.Fatalf("Failed to create TC: %v", err)
	}
	return tc
}

// TODO(meling): Currently only used from disabledTestConvertTimeoutCertBLS12.
// Ideally, we should replace CreateTC above with this function since it avoids two arguments with the same signers.
func CreateTCOld(t *testing.T, view hotstuff.View, signers []*cert.Authority) hotstuff.TimeoutCert {
	t.Helper()
	if len(signers) == 0 {
		return hotstuff.TimeoutCert{}
	}
	x := signers[0]
	tc, err := x.CreateTimeoutCert(view, CreateTimeouts(t, view, signers))
	if err != nil {
		t.Fatalf("Failed to create TC: %v", err)
	}
	return tc
}

// GenerateECDSAKey generates an ECDSA private key for use in tests.
func GenerateECDSAKey(t *testing.T) hotstuff.PrivateKey {
	t.Helper()
	key, err := keygen.GenerateECDSAPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}

// GenerateEDDSAKey generates an ECDSA private key for use in tests.
func GenerateEDDSAKey(t *testing.T) hotstuff.PrivateKey {
	t.Helper()
	_, key, err := keygen.GenerateED25519Key()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}

// GenerateBLS12Key generates a BLS12-381 private key for use in tests.
func GenerateBLS12Key(t *testing.T) hotstuff.PrivateKey {
	t.Helper()
	key, err := bls12.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}
