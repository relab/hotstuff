package crypto_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/crypto"
)

// TestBLS12SingleSignerVerify tests that a single-signer BLS12 signature
// can be verified correctly. This test ensures that the Verify function
// returns early after verifying a single-signer signature, preventing
// unnecessary double verification that could cause intermittent failures.
func TestBLS12SingleSignerVerify(t *testing.T) {
	// Generate a private key for BLS12
	priv, err := crypto.GenerateBLS12PrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	
	// Create runtime config with the private key
	cfg := core.NewRuntimeConfig(1, priv)
	cfg.AddReplica(&hotstuff.ReplicaInfo{
		ID:       1,
		PubKey:   priv.Public(),
		Metadata: map[string]string{},
	})
	
	// Create BLS12 crypto instance
	bls, err := crypto.NewBLS12(cfg)
	if err != nil {
		t.Fatalf("Failed to create BLS12: %v", err)
	}
	
	testCases := []struct {
		name    string
		message string
	}{
		{name: "SimpleMessage", message: "test message"},
		{name: "EmptyMessage", message: ""},
		{name: "LongMessage", message: "this is a very long message that needs to be signed and verified using BLS12-381"},
		{name: "BinaryData", message: string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD})},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := bls.Sign([]byte(tc.message))
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}
			if sig == nil {
				t.Fatal("signature is nil")
			}
			// Verify the signature - this should take the single-signer path
			if err = bls.Verify(sig, []byte(tc.message)); err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}

// TestBLS12MultiSignerVerify tests that multi-signer BLS12 aggregate signatures
// can be verified correctly.
func TestBLS12MultiSignerVerify(t *testing.T) {
	testCases := []struct {
		name        string
		numReplicas int
		message     string
	}{
		{name: "TwoSigners", numReplicas: 2, message: "test message"},
		{name: "FourSigners", numReplicas: 4, message: "test message"},
		{name: "TenSigners", numReplicas: 10, message: "test message"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate keys and configs for all replicas
			keys := make(map[hotstuff.ID]hotstuff.PrivateKey)
			configs := make(map[hotstuff.ID]*core.RuntimeConfig)
			
			// First pass: generate keys and create initial configs
			for i := 1; i <= tc.numReplicas; i++ {
				id := hotstuff.ID(i)
				priv, err := crypto.GenerateBLS12PrivateKey()
				if err != nil {
					t.Fatalf("Failed to generate private key: %v", err)
				}
				keys[id] = priv
				configs[id] = core.NewRuntimeConfig(id, priv)
			}
			
			// Second pass: create BLS12 instances to get PoP metadata
			tempBLS := make(map[hotstuff.ID]crypto.Base)
			for i := 1; i <= tc.numReplicas; i++ {
				id := hotstuff.ID(i)
				bls, err := crypto.NewBLS12(configs[id])
				if err != nil {
					t.Fatalf("Failed to create BLS12: %v", err)
				}
				tempBLS[id] = bls
			}
			
			// Third pass: recreate configs with PoP metadata and create final instances
			blsInstances := make([]crypto.Base, 0, tc.numReplicas)
			for i := 1; i <= tc.numReplicas; i++ {
				id := hotstuff.ID(i)
				cfg := core.NewRuntimeConfig(id, keys[id])
				
				// Add all replicas with their PoP metadata
				for j := 1; j <= tc.numReplicas; j++ {
					jid := hotstuff.ID(j)
					replica := &hotstuff.ReplicaInfo{
						ID:       jid,
						PubKey:   keys[jid].Public(),
						Metadata: configs[jid].ConnectionMetadata(),
					}
					cfg.AddReplica(replica)
				}
				
				bls, err := crypto.NewBLS12(cfg)
				if err != nil {
					t.Fatalf("Failed to recreate BLS12 with PoP: %v", err)
				}
				blsInstances = append(blsInstances, bls)
			}
			
			// Create individual signatures
			var signatures []hotstuff.QuorumSignature
			for _, bls := range blsInstances {
				sig, err := bls.Sign([]byte(tc.message))
				if err != nil {
					t.Fatalf("Sign failed: %v", err)
				}
				signatures = append(signatures, sig)
			}
			
			// Combine signatures
			combinedSig, err := blsInstances[0].Combine(signatures...)
			if err != nil {
				t.Fatalf("Combine failed: %v", err)
			}
			
			// Verify the aggregate signature - this should take the multi-signer path
			if err = blsInstances[0].Verify(combinedSig, []byte(tc.message)); err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
		})
	}
}
