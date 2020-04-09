package hotstuff

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"
)

// PartialSig is a single replica's signature of a node.
type PartialSig struct {
	ID   ReplicaID
	R, S *big.Int
}

func (psig PartialSig) toBytes() []byte {
	var b []byte
	i := make([]byte, 4)
	binary.LittleEndian.PutUint32(i, uint32(psig.ID))
	b = append(b, i...)
	b = append(b, psig.R.Bytes()...)
	b = append(b, psig.S.Bytes()...)
	return b
}

// PartialCert is a single replica's certificate for a node.
type PartialCert struct {
	Sig      PartialSig
	NodeHash NodeHash
}

// QuorumCert is a certificate for a node from a quorum of replicas.
type QuorumCert struct {
	Sigs     map[ReplicaID]PartialSig
	NodeHash NodeHash
}

func (qc *QuorumCert) toBytes() []byte {
	var b []byte
	b = append(b, qc.NodeHash[:]...)
	// sort partial signatures into a slice to ensure determinism
	// TODO: find out if there is a faster way to ensure this
	psigs := make([]PartialSig, 0, len(qc.Sigs))
	for _, v := range qc.Sigs {
		psigs = append(psigs, v)
	}
	sort.SliceStable(psigs, func(i, j int) bool {
		return psigs[i].ID < psigs[j].ID
	})
	for i := range psigs {
		b = append(b, psigs[i].toBytes()...)
	}
	return b
}

func (qc *QuorumCert) String() string {
	return fmt.Sprintf("QuorumCert{Sigs: %d, Hash: %.8s}", len(qc.Sigs), qc.NodeHash)
}

// AddPartial adds the partial signature to the quorum cert.
func (qc *QuorumCert) AddPartial(cert *PartialCert) error {
	// dont add a cert if there is already a signature from the same replica
	if _, exists := qc.Sigs[cert.Sig.ID]; exists {
		return fmt.Errorf("Attempt to add partial cert from same replica twice")
	}

	if !bytes.Equal(qc.NodeHash[:], cert.NodeHash[:]) {
		return fmt.Errorf("Partial cert hash does not match quorum cert")
	}

	qc.Sigs[cert.Sig.ID] = cert.Sig

	return nil
}

// CreatePartialCert creates a partial cert from a node.
func CreatePartialCert(id ReplicaID, privKey *ecdsa.PrivateKey, node *Node) (*PartialCert, error) {
	hash := node.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	if err != nil {
		return nil, err
	}
	sig := PartialSig{id, r, s}
	return &PartialCert{sig, hash}, nil
}

// VerifyPartialCert will verify a PartialCert from a public key stored in ReplicaConfig
func VerifyPartialCert(conf *ReplicaConfig, cert *PartialCert) bool {
	info, ok := conf.Replicas[cert.Sig.ID]
	if !ok {
		logger.Printf("VerifyPartialSig: got signature from replica whose ID (%d) was not in config.", cert.Sig.ID)
		return false
	}
	return ecdsa.Verify(info.PubKey, cert.NodeHash[:], cert.Sig.R, cert.Sig.S)
}

// CreateQuorumCert creates an empty quorum certificate for a given node
func CreateQuorumCert(node *Node) *QuorumCert {
	return &QuorumCert{NodeHash: node.Hash(), Sigs: make(map[ReplicaID]PartialSig)}
}

// VerifyQuorumCert will verify a QuorumCert from public keys stored in ReplicaConfig
func VerifyQuorumCert(conf *ReplicaConfig, qc *QuorumCert) bool {
	if len(qc.Sigs) < conf.QuorumSize {
		return false
	}
	numVerified := 0
	for _, psig := range qc.Sigs {
		info, ok := conf.Replicas[psig.ID]
		if !ok {
			logger.Printf("VerifyQuorumSig: got signature from replica whose ID (%d) was not in config.", psig.ID)
		}

		if ecdsa.Verify(info.PubKey, qc.NodeHash[:], psig.R, psig.S) {
			numVerified++
		}
	}
	return numVerified >= conf.QuorumSize
}
