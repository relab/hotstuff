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
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	ID         ReplicaID
	PrivateKey *ecdsa.PrivateKey
	Cert       *tls.Certificate // Own certificate
	CertPool   *x509.CertPool   // Other replicas's certificates
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
		CertPool:   x509.NewCertPool(),
		Replicas:   make(map[ReplicaID]*ReplicaInfo),
		BatchSize:  1,
	}
}
