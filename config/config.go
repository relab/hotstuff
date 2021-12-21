// Package config contains structs that are useful for initializing consensus.
//
// These structs do not implement the consensus.Replica or consensus.Configuration interfaces,
// but do contain more or less the same information.
package config

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"google.golang.org/grpc/credentials"
)

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	ID      hotstuff.ID
	Address string
	PubKey  consensus.PublicKey
}

// ReplicaConfig holds information needed by a replica.
type ReplicaConfig struct {
	ID         hotstuff.ID
	PrivateKey consensus.PrivateKey
	Creds      credentials.TransportCredentials
	Replicas   map[hotstuff.ID]*ReplicaInfo
}

// NewConfig returns a new ReplicaConfig instance.
func NewConfig(id hotstuff.ID, privateKey consensus.PrivateKey, creds credentials.TransportCredentials) *ReplicaConfig {
	return &ReplicaConfig{
		ID:         id,
		PrivateKey: privateKey,
		Creds:      creds,
		Replicas:   make(map[hotstuff.ID]*ReplicaInfo),
	}
}
