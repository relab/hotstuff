package hotstuff

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
)

// partialSig is a single replica's signature of a node.
type partialSig struct {
	id   ReplicaID
	r, s *big.Int
}

// PartialCert is a single replica's certificate for a node.
type PartialCert struct {
	sig  partialSig
	hash []byte
}

// QuorumCert is a certificate for a node from a quorum of replicas.
type QuorumCert struct {
	mut  sync.Mutex
	sigs []partialSig
	hash []byte
}

// AddPartial adds the partial signature to the quorum cert.
func (qc *QuorumCert) AddPartial(cert PartialCert) error {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	if !bytes.Equal(qc.hash, cert.hash) {
		return fmt.Errorf("Partial cert hash does not match quorum cert")
	}

	qc.sigs = append(qc.sigs, cert.sig)

	return nil
}

// CreatePartialCert creates a partial cert from a node.
func CreatePartialCert(id ReplicaID, privKey *ecdsa.PrivateKey, node *Node) (*PartialCert, error) {
	hash := node.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash)
	if err != nil {
		return nil, err
	}
	sig := partialSig{id, r, s}
	return &PartialCert{sig, hash}, nil
}

// VerifyPartialCert will verify a PartialCert from a public key stored in ReplicaConfig
func VerifyPartialCert(conf ReplicaConfig, cert *PartialCert) bool {
	info, ok := conf.Replicas[cert.sig.id]
	if !ok {
		logger.Printf("VerifyPartialSig: got signature from replica whose ID (%d) was not in config.", cert.sig.id)
		return false
	}
	return ecdsa.Verify(info.PubKey, cert.hash, cert.sig.s, cert.sig.s)
}

// VerifyQuorumCert will verify a QuorumCert from public keys stored in ReplicaConfig
func VerifyQuorumCert(conf ReplicaConfig, qc QuorumCert) bool {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	if len(qc.sigs) < conf.Majority {
		return false
	}
	numVerified := 0
	for _, psig := range qc.sigs {
		info, ok := conf.Replicas[psig.id]
		if !ok {
			logger.Printf("VerifyQuorumSig: got signature from replica whose ID (%d) was not in config.", psig.id)
		}

		if ecdsa.Verify(info.PubKey, qc.hash, psig.r, psig.s) {
			numVerified++
		}
	}
	return numVerified >= conf.Majority
}
