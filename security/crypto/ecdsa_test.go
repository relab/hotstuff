package crypto_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"maps"
	"slices"
	"strings"
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
		sigIndices  []int  // indices of signers to sign (repeat for overlap)
		badType     bool   // include an incompatible type to trigger type error
		wantErr     string // expected error substring
	}{
		{name: "TwoSignatures", numReplicas: 4, sigIndices: []int{0, 1}, wantErr: ""},
		{name: "FourSignatures", numReplicas: 4, sigIndices: []int{0, 1, 2, 3}, wantErr: ""},
		{name: "UnorderedFiveSignatures", numReplicas: 5, sigIndices: []int{3, 2, 1, 0, 4}, wantErr: ""},
		{name: "SingleSignature", numReplicas: 2, sigIndices: []int{0}, wantErr: "must have at least two signatures"},
		{name: "DuplicateSignatures", numReplicas: 3, sigIndices: []int{0, 0, 1}, wantErr: "overlapping signatures"},
		{name: "IncompatibleType", numReplicas: 2, sigIndices: []int{0, 1}, badType: true, wantErr: "incompatible type"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signers := setupECDSATestMulti(t, tc.numReplicas)

			message := []byte("combine message")
			// Create signatures from specified replicas (repeat index for overlap)
			var sigs []hotstuff.QuorumSignature
			for _, idx := range tc.sigIndices {
				sig, err := signers[idx].Sign(message)
				if err != nil {
					t.Fatalf("Sign failed for replica %d: %v", idx+1, err)
				}
				sigs = append(sigs, sig)
			}

			if tc.badType {
				// Append an incompatible hotstuff.QuorumSignature type that
				// isn't Multi[*ECDSASignature] to trigger type error in Combine
				sigs = append(sigs, dummyQuorumSignature{})
			}

			ec := signers[0]
			combined, err := ec.Combine(sigs...)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Combine() error: %q, want %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Combine failed: %v", err)
			}

			if err = ec.Verify(combined, message); err != nil {
				t.Fatalf("Verify combined signature failed: %v", err)
			}

			// Check that the combined signature has the correct number of participants
			if combined.Participants().Len() != len(tc.sigIndices) {
				t.Fatalf("expected %d participants, got %d", len(tc.sigIndices), combined.Participants().Len())
			}
		})
	}
}

// dummyQuorumSignature is a minimal implementation of hotstuff.QuorumSignature that is
// intentionally not a Multi[*ECDSASignature], used to test type errors in Combine.
type dummyQuorumSignature struct{}

func (dummyQuorumSignature) Participants() hotstuff.IDSet { return nil }
func (dummyQuorumSignature) ToBytes() []byte              { return nil }

func TestECDSABatchVerify(t *testing.T) {
	testCases := []struct {
		name        string
		numReplicas int
		messages    map[hotstuff.ID][]byte
		rcvMessages map[hotstuff.ID][]byte
		wantErr     string
	}{
		{
			name:        "TwoReplicasDifferentMessages",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message for replica 1"),
				2: []byte("message for replica 2"),
			},
			wantErr: "",
		},
		{
			name:        "FourReplicasDifferentMessages",
			numReplicas: 4,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
				3: []byte("message 3"),
				4: []byte("message 4"),
			},
			wantErr: "",
		},
		{
			name:        "DuplicateMessages",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("same message"),
				2: []byte("same message"),
			},
			wantErr: "ecdsa: invalid signature",
		},
		{
			name:        "MissingMessage",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
			},
			rcvMessages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				// missing message for replica 2, but got its signature
			},
			wantErr: "ecdsa: message not found",
		},
		{
			name:        "ExtraMessage",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
			},
			rcvMessages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
				3: []byte("message 3"),
			},
			wantErr: "ecdsa: invalid signature",
		},
		{
			name:        "MessageFromUnknownReplica",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
			},
			rcvMessages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				3: []byte("message 3"),
			},
			wantErr: "ecdsa: message not found",
		},
		{
			name:        "TamperedMessage",
			numReplicas: 2,
			messages: map[hotstuff.ID][]byte{
				1: []byte("message 1"),
				2: []byte("message 2"),
			},
			rcvMessages: map[hotstuff.ID][]byte{
				1: []byte("message X"),
				2: []byte("message 2"),
			},
			wantErr: "ecdsa: failed to verify signature from replica 1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signers := setupECDSATestMulti(t, tc.numReplicas)

			// Create signatures for the message batch in deterministic order
			sigs := make([]hotstuff.QuorumSignature, len(tc.messages))
			for _, id := range slices.Sorted(maps.Keys(tc.messages)) {
				msg := tc.messages[id]
				sig, err := signers[id-1].Sign(msg)
				if err != nil {
					t.Errorf("Sign failed for replica %d: %v", id, err)
				}
				sigs[id-1] = sig
			}

			// Combine all signatures
			ec := signers[0]
			combined, err := ec.Combine(sigs...)
			if err != nil {
				t.Errorf("Combine failed: %v", err)
			}

			// Use tampered messages if provided
			msgs := tc.messages
			if tc.rcvMessages != nil {
				msgs = tc.rcvMessages
			}

			// Verify the batch of messages against the combined quorum signature
			err = ec.BatchVerify(combined, msgs)
			if err != nil {
				if err.Error() != tc.wantErr {
					t.Errorf("BatchVerify() error: %q, want %q", err.Error(), tc.wantErr)
				}
				if tc.wantErr == "" {
					t.Errorf("BatchVerify() error: %v, want <nil>", err)
				}
			}
			if err == nil && tc.wantErr != "" {
				t.Errorf("BatchVerify() error: <nil>, want %q", tc.wantErr)
			}
		})
	}
}

func TestECDSASignatureToBytes(t *testing.T) {
	ec := setupECDSATest(t, 1)
	message := []byte("test message")
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Extract the ECDSA signature from Multi signature
	ecdsaSig := sig.(crypto.Multi[*crypto.ECDSASignature])[0]

	sigBytes := ecdsaSig.ToBytes()
	if len(sigBytes) == 0 {
		t.Error("ToBytes() returned empty slice")
	}
	if ecdsaSig.Signer() != 1 {
		t.Errorf("Expected signer ID 1, got %d", ecdsaSig.Signer())
	}
}

func TestECDSARestoreSignature(t *testing.T) {
	ec := setupECDSATest(t, 1)
	message := []byte("test message")
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Extract the ECDSA signature from Multi signature
	originalSig := sig.(crypto.Multi[*crypto.ECDSASignature])[0]
	originalSigBytes := originalSig.ToBytes()
	signer := originalSig.Signer()

	// Restore ECDSA signature using the original bytes and signer
	restoredSig := crypto.RestoreECDSASignature(originalSigBytes, signer)

	// Verify that the restored signature has the same bytes as the original
	if !bytes.Equal(originalSigBytes, restoredSig.ToBytes()) {
		t.Fatal("Restored signature bytes do not match original")
	}
	if restoredSig.Signer() != signer {
		t.Fatalf("Restored signer does not match: got %v, want %v", restoredSig.Signer(), signer)
	}

	// Create a Multi signature from the restored ECDSA signature
	restoredMulti := crypto.NewMulti(restoredSig)

	// Check that the restored signature can be verified
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
