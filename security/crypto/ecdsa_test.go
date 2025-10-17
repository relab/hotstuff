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
func setupECDSATest(t *testing.T, numReplicas int) (*crypto.ECDSA, map[hotstuff.ID]*ecdsa.PrivateKey) {
	t.Helper()
	
	keys := make(map[hotstuff.ID]*ecdsa.PrivateKey)
	
	for i := 1; i <= numReplicas; i++ {
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate key for replica %d: %v", i, err)
		}
		id := hotstuff.ID(i)
		keys[id] = privKey
	}
	
	// Use replica 1 as the signer for this test
	config := core.NewRuntimeConfig(hotstuff.ID(1), keys[1])
	
	// Add all replicas to the configuration
	for i := 1; i <= numReplicas; i++ {
		id := hotstuff.ID(i)
		config.AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: &keys[id].PublicKey,
		})
	}
	
	return crypto.NewECDSA(config), keys
}

func TestECDSASignAndVerify(t *testing.T) {
	testCases := []struct {
		name       string
		message    string
		numReplicas int
	}{
		{
			name:       "SingleReplica",
			message:    "test message",
			numReplicas: 1,
		},
		{
			name:       "EmptyMessage",
			message:    "",
			numReplicas: 1,
		},
		{
			name:       "LongMessage",
			message:    "this is a very long message that needs to be signed and verified using ECDSA",
			numReplicas: 1,
		},
		{
			name:       "BinaryData",
			message:    string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}),
			numReplicas: 1,
		},
		{
			name:       "MultipleReplicas",
			message:    "test with multiple replicas",
			numReplicas: 4,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ec, _ := setupECDSATest(t, tc.numReplicas)
			
			// Sign the message
			sig, err := ec.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			
			if sig == nil {
				t.Fatal("signature is nil")
			}
			
			// Verify the signature
			err = ec.Verify(sig, []byte(tc.message))
			if err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}

func TestECDSAVerifyFailure(t *testing.T) {
	testCases := []struct {
		name          string
		message       string
		tamperedMsg   string
		expectedError string
	}{
		{
			name:          "ModifiedMessage",
			message:       "original message",
			tamperedMsg:   "tampered message",
			expectedError: "failed to verify signature",
		},
		{
			name:          "EmptyMessageTampering",
			message:       "message",
			tamperedMsg:   "",
			expectedError: "failed to verify signature",
		},
		{
			name:          "OneByteChange",
			message:       "message",
			tamperedMsg:   "messag",
			expectedError: "failed to verify signature",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ec, _ := setupECDSATest(t, 1)
			
			// Sign the original message
			sig, err := ec.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			
			// Try to verify with tampered message
			err = ec.Verify(sig, []byte(tc.tamperedMsg))
			if err == nil {
				t.Fatal("Verify should have failed with tampered message")
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
			// Create multiple ECDSA instances for different replicas
			keys := make(map[hotstuff.ID]*ecdsa.PrivateKey)
			
			for i := 1; i <= tc.numReplicas; i++ {
				privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatalf("failed to generate key for replica %d: %v", i, err)
				}
				id := hotstuff.ID(i)
				keys[id] = privKey
			}
			
			// Create a shared config with all replicas
			config := core.NewRuntimeConfig(hotstuff.ID(1), keys[1])
			for i := 1; i <= tc.numReplicas; i++ {
				id := hotstuff.ID(i)
				config.AddReplica(&hotstuff.ReplicaInfo{
					ID:     id,
					PubKey: &keys[id].PublicKey,
				})
			}
			
			message := []byte("test message for combining")
			sigs := make([]hotstuff.QuorumSignature, 0, tc.numSigs)
			
			// Create signatures from different replicas
			for i := 1; i <= tc.numSigs; i++ {
				replicaConfig := core.NewRuntimeConfig(hotstuff.ID(i), keys[hotstuff.ID(i)])
				for j := 1; j <= tc.numReplicas; j++ {
					id := hotstuff.ID(j)
					replicaConfig.AddReplica(&hotstuff.ReplicaInfo{
						ID:     id,
						PubKey: &keys[id].PublicKey,
					})
				}
				ec := crypto.NewECDSA(replicaConfig)
				sig, err := ec.Sign(message)
				if err != nil {
					t.Fatalf("Sign failed for replica %d: %v", i, err)
				}
				sigs = append(sigs, sig)
			}
			
			// Try to combine signatures
			ec := crypto.NewECDSA(config)
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
			// Create multiple ECDSA instances for different replicas
			keys := make(map[hotstuff.ID]*ecdsa.PrivateKey)
			
			for i := 1; i <= tc.numReplicas; i++ {
				privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatalf("failed to generate key for replica %d: %v", i, err)
				}
				id := hotstuff.ID(i)
				keys[id] = privKey
			}
			
			// Create a shared config
			config := core.NewRuntimeConfig(hotstuff.ID(1), keys[1])
			for i := 1; i <= tc.numReplicas; i++ {
				id := hotstuff.ID(i)
				config.AddReplica(&hotstuff.ReplicaInfo{
					ID:     id,
					PubKey: &keys[id].PublicKey,
				})
			}
			
			// Create signatures for the batch
			batch := make(map[hotstuff.ID][]byte)
			sigs := make([]hotstuff.QuorumSignature, 0, len(tc.messages))
			
			for id, msg := range tc.messages {
				batch[id] = []byte(msg)
				replicaConfig := core.NewRuntimeConfig(id, keys[id])
				for i := 1; i <= tc.numReplicas; i++ {
					rid := hotstuff.ID(i)
					replicaConfig.AddReplica(&hotstuff.ReplicaInfo{
						ID:     rid,
						PubKey: &keys[rid].PublicKey,
					})
				}
				ec := crypto.NewECDSA(replicaConfig)
				sig, err := ec.Sign([]byte(msg))
				if err != nil {
					t.Fatalf("Sign failed for replica %d: %v", id, err)
				}
				sigs = append(sigs, sig)
			}
			
			// Combine all signatures
			ec := crypto.NewECDSA(config)
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

func TestECDSASignatureRSMethods(t *testing.T) {
	ec, _ := setupECDSATest(t, 1)
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
	
	// Test R() and S() methods
	r := ecdsaSig.R()
	s := ecdsaSig.S()
	
	if r == nil {
		t.Fatal("R() returned nil")
	}
	if s == nil {
		t.Fatal("S() returned nil")
	}
	
	// R and S should be positive
	if r.Sign() <= 0 {
		t.Fatalf("R should be positive, got %v", r)
	}
	if s.Sign() <= 0 {
		t.Fatalf("S should be positive, got %v", s)
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
	ec, _ := setupECDSATest(t, 1)
	message := []byte("test message")
	
	// Sign the message
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	
	// Extract R and S from the signature
	multiSig := sig.(crypto.Multi[*crypto.ECDSASignature])
	var originalSig *crypto.ECDSASignature
	for _, s := range multiSig {
		originalSig = s
		break
	}
	
	r := originalSig.R()
	s := originalSig.S()
	signer := originalSig.Signer()
	
	// Restore the signature using R and S
	restoredSig := crypto.RestoreECDSASignature(r, s, signer)
	
	// Verify that the restored signature has the same R, S values
	if restoredR := restoredSig.R(); restoredR.Cmp(r) != 0 {
		t.Fatalf("restored R does not match: got %v, want %v", restoredR, r)
	}
	if restoredS := restoredSig.S(); restoredS.Cmp(s) != 0 {
		t.Fatalf("restored S does not match: got %v, want %v", restoredS, s)
	}
	if restoredSig.Signer() != signer {
		t.Fatalf("restored signer does not match: got %v, want %v", restoredSig.Signer(), signer)
	}
	
	// Create a Multi signature with the restored signature
	restoredMulti := crypto.Multi[*crypto.ECDSASignature]{signer: restoredSig}
	
	// Verify that the restored signature can be verified
	err = ec.Verify(restoredMulti, message)
	if err != nil {
		t.Fatalf("Verify restored signature failed: %v", err)
	}
}

func TestECDSASignatureASN1Encoding(t *testing.T) {
	// This test verifies that signatures use ASN.1 encoding internally
	ec, _ := setupECDSATest(t, 1)
	message := []byte("test message")
	
	// Sign the message
	sig, err := ec.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	
	// Extract the signature
	multiSig := sig.(crypto.Multi[*crypto.ECDSASignature])
	var ecdsaSig *crypto.ECDSASignature
	for _, s := range multiSig {
		ecdsaSig = s
		break
	}
	
	// Get the ASN.1 encoded signature bytes
	sigBytes := ecdsaSig.ToBytes()
	
	// ASN.1 encoded ECDSA signatures should start with 0x30 (SEQUENCE tag)
	// Minimum length is 8 bytes: 1 byte SEQUENCE tag + 1 byte length + 
	// 2 bytes for R (1 byte INTEGER tag + 1 byte length) + 
	// 2 bytes for S (1 byte INTEGER tag + 1 byte length) + 
	// 2 bytes minimum for actual R and S values
	const minASN1SignatureLength = 8
	if len(sigBytes) < minASN1SignatureLength {
		t.Fatalf("signature too short: %d bytes (minimum %d)", len(sigBytes), minASN1SignatureLength)
	}
	if sigBytes[0] != 0x30 {
		t.Fatalf("signature does not start with ASN.1 SEQUENCE tag (0x30), got 0x%02x", sigBytes[0])
	}
	
	// Verify the signature still works
	err = ec.Verify(sig, message)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestECDSAWithDifferentCurves(t *testing.T) {
	curves := []struct {
		name  string
		curve elliptic.Curve
	}{
		{"P256", elliptic.P256()},
		{"P384", elliptic.P384()},
		{"P521", elliptic.P521()},
	}
	
	for _, tc := range curves {
		t.Run(tc.name, func(t *testing.T) {
			// Generate key with specific curve
			privKey, err := ecdsa.GenerateKey(tc.curve, rand.Reader)
			if err != nil {
				t.Fatalf("failed to generate key: %v", err)
			}
			
			config := core.NewRuntimeConfig(hotstuff.ID(1), privKey)
			config.AddReplica(&hotstuff.ReplicaInfo{
				ID:     1,
				PubKey: &privKey.PublicKey,
			})
			
			ec := crypto.NewECDSA(config)
			
			message := []byte("test message")
			
			// Sign
			sig, err := ec.Sign(message)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			
			// Verify
			err = ec.Verify(sig, message)
			if err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}

func TestECDSASignatureDeterminism(t *testing.T) {
	// While ECDSA signatures are randomized and will differ between calls,
	// we test that they are all valid
	ec, _ := setupECDSATest(t, 1)
	message := []byte("test message")
	
	numSignatures := 10
	signatures := make([]hotstuff.QuorumSignature, numSignatures)
	
	// Generate multiple signatures
	for i := 0; i < numSignatures; i++ {
		sig, err := ec.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed on iteration %d: %v", i, err)
		}
		signatures[i] = sig
		
		// Each signature should verify independently
		err = ec.Verify(sig, message)
		if err != nil {
			t.Fatalf("Verify failed on iteration %d: %v", i, err)
		}
	}
	
	// The signatures should differ (ECDSA is randomized)
	// Compare first two signatures
	sig1 := signatures[0].(crypto.Multi[*crypto.ECDSASignature])
	sig2 := signatures[1].(crypto.Multi[*crypto.ECDSASignature])
	
	var bytes1, bytes2 []byte
	for _, s := range sig1 {
		bytes1 = s.ToBytes()
		break
	}
	for _, s := range sig2 {
		bytes2 = s.ToBytes()
		break
	}
	
	// Note: There's a very small chance they could be equal, but it's extremely unlikely
	// This is just to document the randomized nature of ECDSA
	allEqual := true
	if len(bytes1) != len(bytes2) {
		allEqual = false
	} else {
		for i := range bytes1 {
			if bytes1[i] != bytes2[i] {
				allEqual = false
				break
			}
		}
	}
	
	if allEqual {
		t.Log("Note: Two signatures were identical (very rare but possible)")
	}
}

func BenchmarkECDSASign(b *testing.B) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("failed to generate key: %v", err)
	}
	
	config := core.NewRuntimeConfig(hotstuff.ID(1), privKey)
	config.AddReplica(&hotstuff.ReplicaInfo{
		ID:     1,
		PubKey: &privKey.PublicKey,
	})
	
	ec := crypto.NewECDSA(config)
	message := []byte("benchmark message")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ec.Sign(message)
		if err != nil {
			b.Fatalf("Sign failed: %v", err)
		}
	}
}

func BenchmarkECDSAVerify(b *testing.B) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("failed to generate key: %v", err)
	}
	
	config := core.NewRuntimeConfig(hotstuff.ID(1), privKey)
	config.AddReplica(&hotstuff.ReplicaInfo{
		ID:     1,
		PubKey: &privKey.PublicKey,
	})
	
	ec := crypto.NewECDSA(config)
	message := []byte("benchmark message")
	
	sig, err := ec.Sign(message)
	if err != nil {
		b.Fatalf("Sign failed: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := ec.Verify(sig, message)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}

func BenchmarkECDSACombine(b *testing.B) {
	numReplicas := 4
	keys := make(map[hotstuff.ID]*ecdsa.PrivateKey)
	
	for i := 1; i <= numReplicas; i++ {
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			b.Fatalf("failed to generate key: %v", err)
		}
		id := hotstuff.ID(i)
		keys[id] = privKey
	}
	
	// Create a shared config
	config := core.NewRuntimeConfig(hotstuff.ID(1), keys[1])
	for i := 1; i <= numReplicas; i++ {
		id := hotstuff.ID(i)
		config.AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: &keys[id].PublicKey,
		})
	}
	
	message := []byte("benchmark message")
	sigs := make([]hotstuff.QuorumSignature, numReplicas)
	
	for i := 1; i <= numReplicas; i++ {
		replicaConfig := core.NewRuntimeConfig(hotstuff.ID(i), keys[hotstuff.ID(i)])
		for j := 1; j <= numReplicas; j++ {
			id := hotstuff.ID(j)
			replicaConfig.AddReplica(&hotstuff.ReplicaInfo{
				ID:     id,
				PubKey: &keys[id].PublicKey,
			})
		}
		ec := crypto.NewECDSA(replicaConfig)
		sig, err := ec.Sign(message)
		if err != nil {
			b.Fatalf("Sign failed: %v", err)
		}
		sigs[i-1] = sig
	}
	
	ec := crypto.NewECDSA(config)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ec.Combine(sigs...)
		if err != nil {
			b.Fatalf("Combine failed: %v", err)
		}
	}
}
