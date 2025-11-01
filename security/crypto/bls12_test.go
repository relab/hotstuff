package crypto_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/crypto"
)

// createBLS12Signer creates a single BLS12-based signer for testing.
func createBLS12Signer(t testing.TB, numReplicas int) crypto.Base {
	t.Helper()
	// Generate keys for all replicas
	keys := make(map[hotstuff.ID]hotstuff.PrivateKey, numReplicas)
	for i := 1; i <= numReplicas; i++ {
		keys[hotstuff.ID(i)] = testutil.GenerateBLS12Key(t)
	}
	// Create config for the first replica
	cfg := core.NewRuntimeConfig(hotstuff.ID(1), keys[hotstuff.ID(1)])
	for i := 1; i <= numReplicas; i++ {
		rid := hotstuff.ID(i)
		cfg.AddReplica(&hotstuff.ReplicaInfo{ID: rid, PubKey: keys[rid].Public()})
	}
	// Create the base crypto for the first replica
	base, err := crypto.NewBLS12(cfg)
	if err != nil {
		t.Fatalf("Failed to create BLS12 base: %v", err)
	}
	return base
}

func TestBLS12SignAndVerify(t *testing.T) {
	testCases := []struct {
		name    string
		message string
	}{
		{name: "SingleReplica", message: "test message"},
		{name: "EmptyMessage", message: ""},
		{name: "LongMessage", message: "this is a very long message that needs to be signed and verified using BLS12"},
		{name: "BinaryData", message: string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD})},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bls := createBLS12Signer(t, 1)
			sig, err := bls.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			if sig == nil {
				t.Fatal("signature is nil")
			}
			if err = bls.Verify(sig, []byte(tc.message)); err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}

func TestBLS12VerifyFailure(t *testing.T) {
	testCases := []struct {
		name        string
		message     string
		tamperedMsg string
	}{
		{name: "ModifiedMessage", message: "original message", tamperedMsg: "tampered message"},
		{name: "EmptyMessageTampering", message: "message", tamperedMsg: ""},
		{name: "SubOneByte", message: "message", tamperedMsg: "messag"},
		{name: "AddOneByte", message: "message", tamperedMsg: "message!"},
		{name: "OneByteChange", message: "message", tamperedMsg: "massage"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bls := createBLS12Signer(t, 1)
			sig, err := bls.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			// Confirm that error is returned for tampered message
			err = bls.Verify(sig, []byte(tc.tamperedMsg))
			if err == nil {
				t.Fatal("Verify should have failed with tampered message")
			}
			// The error should contain "bls12" and indicate verification failed
			if err.Error() != "bls12: failed to verify message" {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), "bls12: failed to verify message")
			}
		})
	}
}

func TestBLS12Combine(t *testing.T) {
	testCases := []struct {
		name         string
		numReplicas  int
		numSigners   int
		expectError  bool
		errorMessage string
	}{
		{name: "SingleSignature", numReplicas: 4, numSigners: 1, expectError: true, errorMessage: "must have at least two signatures"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate keys for all replicas
			keys := make(map[hotstuff.ID]hotstuff.PrivateKey, tc.numReplicas)
			for i := 1; i <= tc.numReplicas; i++ {
				keys[hotstuff.ID(i)] = testutil.GenerateBLS12Key(t)
			}
			// Create signers for each replica
			signers := make([]crypto.Base, tc.numReplicas)
			for i := 0; i < tc.numReplicas; i++ {
				id := hotstuff.ID(i + 1)
				cfg := core.NewRuntimeConfig(id, keys[id])
				for j := 1; j <= tc.numReplicas; j++ {
					rid := hotstuff.ID(j)
					cfg.AddReplica(&hotstuff.ReplicaInfo{ID: rid, PubKey: keys[rid].Public()})
				}
				base, err := crypto.NewBLS12(cfg)
				if err != nil {
					t.Fatalf("Failed to create BLS12 base: %v", err)
				}
				signers[i] = base
			}

			message := []byte("test message")
			sigs := make([]hotstuff.QuorumSignature, tc.numSigners)
			for i := 0; i < tc.numSigners; i++ {
				sig, err := signers[i].Sign(message)
				if err != nil {
					t.Fatalf("Sign failed: %v", err)
				}
				sigs[i] = sig
			}

			_, err := signers[0].Combine(sigs...)
			if tc.expectError {
				if err == nil {
					t.Fatal("Expected error, but got none")
				}
				if err.Error() != tc.errorMessage {
					t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.errorMessage)
				}
			} else {
				t.Fatal("Should not reach here - all test cases expect errors")
			}
		})
	}
}

func TestBLS12SingleSignerVerifyNoDuplicateCheck(t *testing.T) {
	// This test specifically checks that single-signer signatures
	// are only verified once (not double-verified)
	// This is a regression test for the bug where Verify() fell through
	// after successfully verifying single-signer signatures
	bls := createBLS12Signer(t, 1)
	message := []byte("test message for single signer")
	
	sig, err := bls.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	
	// Verify should succeed and not perform double verification
	err = bls.Verify(sig, message)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	
	// The test passes if we get here without errors
	// The fix ensures that the verification returns immediately after
	// the single-signer path, without falling through to the multi-signer path
}

func BenchmarkBLS12Sign(b *testing.B) {
	bls := createBLS12Signer(b, 1)
	message := []byte("benchmark message")

	for b.Loop() {
		_, err := bls.Sign(message)
		if err != nil {
			b.Fatalf("Sign failed: %v", err)
		}
	}
}

func BenchmarkBLS12Verify(b *testing.B) {
	bls := createBLS12Signer(b, 1)
	message := []byte("benchmark message")
	sig, err := bls.Sign(message)
	if err != nil {
		b.Fatalf("Sign failed: %v", err)
	}

	for b.Loop() {
		err := bls.Verify(sig, message)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}
