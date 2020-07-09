package config

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
)

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	ID      ReplicaID
	Address string
	PubKey  *ecdsa.PublicKey
	Cert    *x509.Certificate
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	ID         ReplicaID
	PrivateKey *ecdsa.PrivateKey
	Cert       *tls.Certificate
	Replicas   map[ReplicaID]*ReplicaInfo
	QuorumSize int
	BatchSize  int
}

// NewConfig returns a new ReplicaConfig instance
func NewConfig(id ReplicaID, privateKey *ecdsa.PrivateKey, cert *tls.Certificate) *ReplicaConfig {
	return &ReplicaConfig{
		ID:         id,
		PrivateKey: privateKey,
		Cert:       cert,
		Replicas:   make(map[ReplicaID]*ReplicaInfo),
		BatchSize:  1,
	}
}
