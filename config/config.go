package config

import (
	"crypto/ecdsa"

	"google.golang.org/grpc/credentials"
)

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	ID      ReplicaID
	Address string
	PubKey  *ecdsa.PublicKey
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	ID         ReplicaID
	PrivateKey *ecdsa.PrivateKey
	Creds      credentials.TransportCredentials
	Replicas   map[ReplicaID]*ReplicaInfo
	QuorumSize int
	BatchSize  int
}

// NewConfig returns a new ReplicaConfig instance
func NewConfig(id ReplicaID, privateKey *ecdsa.PrivateKey, creds credentials.TransportCredentials) *ReplicaConfig {
	return &ReplicaConfig{
		ID:         id,
		PrivateKey: privateKey,
		Creds:      creds,
		Replicas:   make(map[ReplicaID]*ReplicaInfo),
		BatchSize:  1,
	}
}
