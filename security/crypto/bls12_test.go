package crypto_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/crypto"
)

// createBLS12Signers creates BLS12-based signers for testing.
// Returns a slice of signers, one for each replica.
func createBLS12Signers(t testing.TB, numReplicas int) []crypto.Base {
	t.Helper()
	// Generate keys for all replicas
	keys := make(map[hotstuff.ID]hotstuff.PrivateKey, numReplicas)
	for i := 1; i <= numReplicas; i++ {
		keys[hotstuff.ID(i)] = testutil.GenerateBLS12Key(t)
	}
	
	// Create configs for all replicas
	configs := make([]*core.RuntimeConfig, numReplicas)
	for i := 1; i <= numReplicas; i++ {
		id := hotstuff.ID(i)
		configs[i-1] = core.NewRuntimeConfig(id, keys[id])
		for j := 1; j <= numReplicas; j++ {
			rid := hotstuff.ID(j)
			configs[i-1].AddReplica(&hotstuff.ReplicaInfo{ID: rid, PubKey: keys[rid].Public()})
		}
	}
	
	// Create base crypto instances (this adds connection metadata for each)
	bases := make([]crypto.Base, numReplicas)
	for i := 1; i <= numReplicas; i++ {
		base, err := crypto.NewBLS12(configs[i-1])
		if err != nil {
			t.Fatalf("Failed to create BLS12 base: %v", err)
		}
		bases[i-1] = base
	}
	
	// Share proof-of-possession metadata between all replicas
	for i := 1; i <= numReplicas; i++ {
		srcMeta := configs[i-1].ConnectionMetadata()
		for j := 1; j <= numReplicas; j++ {
			if i != j {
				id := hotstuff.ID(i)
				if err := configs[j-1].SetReplicaMetadata(id, srcMeta); err != nil {
					t.Fatalf("Failed to set replica metadata: %v", err)
				}
			}
		}
	}
	
	return bases
}

// createBLS12Signer creates a single BLS12-based signer for testing.
func createBLS12Signer(t testing.TB, numReplicas int) crypto.Base {
	t.Helper()
	return createBLS12Signers(t, numReplicas)[0]
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
		name       string
		numSigners int
		wantErr    string
	}{
		{name: "SingleSignature", numSigners: 1, wantErr: "must have at least two signatures"},
		{name: "TwoSignatures", numSigners: 2, wantErr: ""},
		{name: "FourSignatures", numSigners: 4, wantErr: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			numReplicas := 4
			signers := createBLS12Signers(t, numReplicas)

			message := []byte("test message")
			sigs := make([]hotstuff.QuorumSignature, tc.numSigners)
			for i := range tc.numSigners {
				sig, err := signers[i].Sign(message)
				if err != nil {
					t.Fatalf("Sign failed: %v", err)
				}
				sigs[i] = sig
			}

			combined, err := signers[0].Combine(sigs...)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("Expected error, but got none")
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Combine failed: %v", err)
				}
				if combined == nil {
					t.Fatal("combined signature is nil")
				}
				// Verify the combined signature
				if err := signers[0].Verify(combined, message); err != nil {
					t.Fatalf("Verify of combined signature failed: %v", err)
				}
			}
		})
	}
}

func TestBLS12SingleSignerVerifyNoDuplicateCheck(t *testing.T) {
	// This is a regression test for the bug where Verify() fell through
	// after successfully verifying single-signer signatures, causing
	// the signature to be verified again via the multi-signer path.
	// The fix ensures that verification returns immediately after the
	// single-signer verification succeeds.
	bls := createBLS12Signer(t, 1)
	message := []byte("test message for single signer")
	
	sig, err := bls.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	
	err = bls.Verify(sig, message)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
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
