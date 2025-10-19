package crypto_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/crypto"
)

// setupECDSATest creates a test configuration with ECDSA keys for testing
func setupECDSATest(t testing.TB, numReplicas int) *crypto.ECDSA {
	t.Helper()
	return setupECDSATestMulti(t, numReplicas)[0]
}

// setupECDSATestMulti creates ECDSA instances for multiple replicas.
// Returns a slice of ECDSA signers (index 0 corresponds to replica 1, etc.)
func setupECDSATestMulti(t testing.TB, numReplicas int) []*crypto.ECDSA {
	t.Helper()

	keys := make(map[hotstuff.ID]*ecdsa.PrivateKey)
	for i := 1; i <= numReplicas; i++ {
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate key for replica %d: %v", i, err)
		}
		keys[hotstuff.ID(i)] = privKey
	}

	signers := make([]*crypto.ECDSA, numReplicas)
	for i := range signers {
		id := hotstuff.ID(i + 1)
		// Create config for this replica (with their key) and add all replicas
		config := core.NewRuntimeConfig(id, keys[id])
		for j := 1; j <= numReplicas; j++ {
			rid := hotstuff.ID(j)
			config.AddReplica(&hotstuff.ReplicaInfo{
				ID:     rid,
				PubKey: &keys[rid].PublicKey,
			})
		}
		signers[i] = crypto.NewECDSA(config)
	}

	return signers
}

func TestECDSASignAndVerify(t *testing.T) {
	testCases := []struct {
		name    string
		message string
	}{
		{name: "SingleReplica", message: "test message"},
		{name: "EmptyMessage", message: ""},
		{name: "LongMessage", message: "this is a very long message that needs to be signed and verified using ECDSA"},
		{name: "BinaryData", message: string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD})},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ec := setupECDSATest(t, 1)
			sig, err := ec.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			if sig == nil {
				t.Fatal("signature is nil")
			}
			if err = ec.Verify(sig, []byte(tc.message)); err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}

func TestECDSAVerifyFailure(t *testing.T) {
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
	wantErr := "ecdsa: failed to verify signature from replica 1"
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ec := setupECDSATest(t, 1)
			sig, err := ec.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			// Confirm that error is returned for tampered message
			err = ec.Verify(sig, []byte(tc.tamperedMsg))
			if err == nil {
				t.Fatal("Verify should have failed with tampered message")
			}
			if err.Error() != wantErr {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), wantErr)
			}
		})
	}
}

func TestECDSACombine(t *testing.T) {
	testCases := []struct {
		name        string
		numReplicas int
		numSigs     int
		shouldFail  bool
		description string
	}{
		{
			name:        "TwoSignatures",
			numReplicas: 4,
			numSigs:     2,
			shouldFail:  false,
			description: "combine two valid signatures",
		},
		{
			name:        "FourSignatures",
			numReplicas: 4,
			numSigs:     4,
			shouldFail:  false,
			description: "combine four valid signatures",
		},
		{
			name:        "SingleSignature",
			numReplicas: 2,
			numSigs:     1,
			shouldFail:  true,
			description: "should fail with only one signature",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signers := setupECDSATestMulti(t, tc.numReplicas)

			message := []byte("test message for combining")
			sigs := make([]hotstuff.QuorumSignature, tc.numSigs)

			// Create signatures from different replicas
			for i := range tc.numSigs {
				sig, err := signers[i].Sign(message)
				if err != nil {
					t.Fatalf("Sign failed for replica %d: %v", i, err)
				}
				sigs[i] = sig
			}

			// Try to combine signatures
			ec := signers[0]
			combined, err := ec.Combine(sigs...)

			if tc.shouldFail {
				if err == nil {
					t.Fatalf("Combine should have failed: %s", tc.description)
				}
				return
			}

			if err != nil {
				t.Fatalf("Combine failed: %v", err)
			}

			// Verify the combined signature
			err = ec.Verify(combined, message)
			if err != nil {
				t.Fatalf("Verify combined signature failed: %v", err)
			}

			// Check that the combined signature has the correct number of participants
			if combined.Participants().Len() != tc.numSigs {
				t.Fatalf("expected %d participants, got %d", tc.numSigs, combined.Participants().Len())
			}
		})
	}
}

func TestECDSABatchVerify(t *testing.T) {
	testCases := []struct {
		name        string
		numReplicas int
		messages    map[hotstuff.ID]string
		shouldFail  bool
		description string
	}{
		{
			name:        "TwoReplicasDifferentMessages",
			numReplicas: 2,
			messages: map[hotstuff.ID]string{
				1: "message for replica 1",
				2: "message for replica 2",
			},
			shouldFail:  false,
			description: "two replicas with different messages",
		},
		{
			name:        "FourReplicasDifferentMessages",
			numReplicas: 4,
			messages: map[hotstuff.ID]string{
				1: "message 1",
				2: "message 2",
				3: "message 3",
				4: "message 4",
			},
			shouldFail:  false,
			description: "four replicas with different messages",
		},
		{
			name:        "DuplicateMessages",
			numReplicas: 2,
			messages: map[hotstuff.ID]string{
				1: "same message",
				2: "same message",
			},
			shouldFail:  true,
			description: "should fail with duplicate messages",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signers := setupECDSATestMulti(t, tc.numReplicas)

			// Create signatures for the batch
			batch := make(map[hotstuff.ID][]byte)
			sigs := make([]hotstuff.QuorumSignature, 0, len(tc.messages))

			for id, msg := range tc.messages {
				batch[id] = []byte(msg)
				sig, err := signers[id-1].Sign([]byte(msg))
				if err != nil {
					t.Fatalf("Sign failed for replica %d: %v", id, err)
				}
				sigs = append(sigs, sig)
			}

			// Combine all signatures
			ec := signers[0]
			combined, err := ec.Combine(sigs...)
			if err != nil {
				t.Fatalf("Combine failed: %v", err)
			}

			// Verify the batch
			err = ec.BatchVerify(combined, batch)

			if tc.shouldFail {
				if err == nil {
					t.Fatalf("BatchVerify should have failed: %s", tc.description)
				}
				return
			}

			if err != nil {
				t.Fatalf("BatchVerify failed: %v", err)
			}
		})
	}
}

func TestECDSASignatureToBytes(t *testing.T) {
	ec := setupECDSATest(t, 1)
	message := []byte("test message")

	// Sign the message
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Extract the signature from Multi
	multiSig := sig.(crypto.Multi[*crypto.ECDSASignature])
	var ecdsaSig *crypto.ECDSASignature
	for _, s := range multiSig {
		ecdsaSig = s
		break
	}

	// Test ToBytes()
	sigBytes := ecdsaSig.ToBytes()
	if len(sigBytes) == 0 {
		t.Fatal("ToBytes() returned empty slice")
	}

	// Test Signer()
	if ecdsaSig.Signer() != 1 {
		t.Fatalf("expected signer ID 1, got %d", ecdsaSig.Signer())
	}
}

func TestECDSARestoreSignature(t *testing.T) {
	ec := setupECDSATest(t, 1)
	message := []byte("test message")

	// Sign the message
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Extract the signature bytes
	multiSig := sig.(crypto.Multi[*crypto.ECDSASignature])
	var originalSig *crypto.ECDSASignature
	for _, s := range multiSig {
		originalSig = s
		break
	}

	sigBytes := originalSig.ToBytes()
	signer := originalSig.Signer()

	// Restore the signature using the bytes
	restoredSig := crypto.RestoreECDSASignature(sigBytes, signer)

	// Verify that the restored signature has the same bytes
	restoredBytes := restoredSig.ToBytes()
	if len(restoredBytes) != len(sigBytes) {
		t.Fatalf("restored signature length does not match: got %d, want %d", len(restoredBytes), len(sigBytes))
	}
	for i := range restoredBytes {
		if restoredBytes[i] != sigBytes[i] {
			t.Fatalf("restored signature bytes do not match at index %d", i)
		}
	}
	if restoredSig.Signer() != signer {
		t.Fatalf("restored signer does not match: got %v, want %v", restoredSig.Signer(), signer)
	}

	// Create a Multi signature with the restored signature
	restoredMulti := crypto.NewMulti(restoredSig)

	// Verify that the restored signature can be verified
	err = ec.Verify(restoredMulti, message)
	if err != nil {
		t.Fatalf("Verify restored signature failed: %v", err)
	}
}

func BenchmarkECDSASign(b *testing.B) {
	ec := setupECDSATest(b, 1)
	message := []byte("benchmark message")

	for b.Loop() {
		_, err := ec.Sign(message)
		if err != nil {
			b.Fatalf("Sign failed: %v", err)
		}
	}
}

func BenchmarkECDSAVerify(b *testing.B) {
	ec := setupECDSATest(b, 1)
	message := []byte("benchmark message")
	sig, err := ec.Sign(message)
	if err != nil {
		b.Fatalf("Sign failed: %v", err)
	}

	for b.Loop() {
		err := ec.Verify(sig, message)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}

func BenchmarkECDSACombine(b *testing.B) {
	numReplicas := 4
	signers := setupECDSATestMulti(b, numReplicas)

	message := []byte("benchmark message")
	sigs := make([]hotstuff.QuorumSignature, numReplicas)
	for i, signer := range signers {
		sig, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("Sign failed: %v", err)
		}
		sigs[i] = sig
	}

	ec := signers[0]

	for b.Loop() {
		_, err := ec.Combine(sigs...)
		if err != nil {
			b.Fatalf("Combine failed: %v", err)
		}
	}
}
