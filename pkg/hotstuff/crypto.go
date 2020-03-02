package hotstuff

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"sort"
	"sync"

	"github.com/relab/hotstuff/pkg/proto"
)

const privateKeyFileType = "HOTSTUFF PRIVATE KEY"
const publicKeyFileType = "HOTSTUFF PUBLIC KEY"

// partialSig is a single replica's signature of a node.
type partialSig struct {
	id   ReplicaID
	r, s *big.Int
}

func (p partialSig) toProto() *proto.PartialSig {
	r := p.r.Bytes()
	s := p.s.Bytes()
	return &proto.PartialSig{
		ReplicaID: int32(p.id),
		R:         r,
		S:         s,
	}
}

func (p partialSig) toBytes() []byte {
	var b []byte
	i := make([]byte, 4)
	binary.LittleEndian.PutUint32(i, uint32(p.id))
	b = append(b, i...)
	b = append(b, p.r.Bytes()...)
	b = append(b, p.s.Bytes()...)
	return b
}

func partialSigFromProto(pps *proto.PartialSig) partialSig {
	r := big.NewInt(0)
	s := big.NewInt(0)
	r.SetBytes(pps.GetR())
	s.SetBytes(pps.GetS())
	return partialSig{
		id: ReplicaID(pps.GetReplicaID()),
		r:  r,
		s:  s,
	}
}

// PartialCert is a single replica's certificate for a node.
type PartialCert struct {
	sig  partialSig
	hash NodeHash
}

func (p PartialCert) toProto() *proto.PartialCert {
	return &proto.PartialCert{
		Sig:  p.sig.toProto(),
		Hash: p.hash[:],
	}
}

func partialCertFromProto(ppc *proto.PartialCert) *PartialCert {
	pc := &PartialCert{
		sig: partialSigFromProto(ppc.GetSig()),
	}
	copy(pc.hash[:], ppc.GetHash())
	return pc
}

// QuorumCert is a certificate for a node from a quorum of replicas.
type QuorumCert struct {
	mut      sync.Mutex
	sigs     map[ReplicaID]partialSig
	NodeHash NodeHash
}

func (qc *QuorumCert) toProto() *proto.QuorumCert {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	sigs := make([]*proto.PartialSig, 0, len(qc.sigs))
	for _, psig := range qc.sigs {
		sigs = append(sigs, psig.toProto())
	}
	return &proto.QuorumCert{
		Sigs: sigs,
		Hash: qc.NodeHash[:],
	}
}

func (qc *QuorumCert) toBytes() []byte {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	var b []byte
	b = append(b, qc.NodeHash[:]...)
	// sort partial signatures into a slice to ensure determinism
	// TODO: find out if there is a faster way to ensure this
	psigs := make([]partialSig, 0, len(qc.sigs))
	for _, v := range qc.sigs {
		psigs = append(psigs, v)
	}
	sort.SliceStable(psigs, func(i, j int) bool {
		return psigs[i].id < psigs[j].id
	})
	for i := range psigs {
		b = append(b, psigs[i].toBytes()...)
	}
	return b
}

func (qc *QuorumCert) String() string {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	return fmt.Sprintf("QuorumCert{Sigs: %d, Hash: %.8s}", len(qc.sigs), qc.NodeHash)
}

func quorumCertFromProto(pqc *proto.QuorumCert) *QuorumCert {
	qc := &QuorumCert{
		sigs: make(map[ReplicaID]partialSig),
	}
	copy(qc.NodeHash[:], pqc.GetHash())
	for _, ppsig := range pqc.GetSigs() {
		psig := partialSigFromProto(ppsig)
		qc.sigs[psig.id] = psig
	}
	return qc
}

// AddPartial adds the partial signature to the quorum cert.
func (qc *QuorumCert) AddPartial(cert *PartialCert) error {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	// dont add a cert if there is already a signature from the same replica
	if _, exists := qc.sigs[cert.sig.id]; exists {
		return fmt.Errorf("Attempt to add partial cert from same replica twice")
	}

	if !bytes.Equal(qc.NodeHash[:], cert.hash[:]) {
		return fmt.Errorf("Partial cert hash does not match quorum cert")
	}

	qc.sigs[cert.sig.id] = cert.sig

	return nil
}

// CreatePartialCert creates a partial cert from a node.
func CreatePartialCert(id ReplicaID, privKey *ecdsa.PrivateKey, node *Node) (*PartialCert, error) {
	hash := node.Hash()
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	if err != nil {
		return nil, err
	}
	sig := partialSig{id, r, s}
	return &PartialCert{sig, hash}, nil
}

// VerifyPartialCert will verify a PartialCert from a public key stored in ReplicaConfig
func VerifyPartialCert(conf *ReplicaConfig, cert *PartialCert) bool {
	info, ok := conf.Replicas[cert.sig.id]
	if !ok {
		logger.Printf("VerifyPartialSig: got signature from replica whose ID (%d) was not in config.", cert.sig.id)
		return false
	}
	return ecdsa.Verify(info.PubKey, cert.hash[:], cert.sig.r, cert.sig.s)
}

// CreateQuorumCert creates an empty quorum certificate for a given node
func CreateQuorumCert(node *Node) *QuorumCert {
	return &QuorumCert{NodeHash: node.Hash(), sigs: make(map[ReplicaID]partialSig)}
}

// VerifyQuorumCert will verify a QuorumCert from public keys stored in ReplicaConfig
func VerifyQuorumCert(conf *ReplicaConfig, qc *QuorumCert) bool {
	qc.mut.Lock()
	defer qc.mut.Unlock()

	if len(qc.sigs) < conf.QuorumSize {
		return false
	}
	numVerified := 0
	for _, psig := range qc.sigs {
		info, ok := conf.Replicas[psig.id]
		if !ok {
			logger.Printf("VerifyQuorumSig: got signature from replica whose ID (%d) was not in config.", psig.id)
		}

		if ecdsa.Verify(info.PubKey, qc.NodeHash[:], psig.r, psig.s) {
			numVerified++
		}
	}
	return numVerified >= conf.QuorumSize
}

// GeneratePrivateKey returns a new public/private key pair based on ECDSA.
func GeneratePrivateKey() (pk *ecdsa.PrivateKey, err error) {
	curve := elliptic.P256()
	pk, err = ecdsa.GenerateKey(curve, rand.Reader)
	return
}

// WritePrivateKeyFile writes a private key to the specified file
func WritePrivateKeyFile(key *ecdsa.PrivateKey, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	b := &pem.Block{
		Type:  privateKeyFileType,
		Bytes: marshalled,
	}

	pem.Encode(f, b)
	return nil
}

// WritePublicKeyFile writes a public key to the specified file
func WritePublicKeyFile(key *ecdsa.PublicKey, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	marshalled, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return err
	}

	b := &pem.Block{
		Type:  publicKeyFileType,
		Bytes: marshalled,
	}

	pem.Encode(f, b)
	return nil
}

// ReadPrivateKeyFile reads a private key from the specified file
func ReadPrivateKeyFile(keyFile string) (key *ecdsa.PrivateKey, err error) {
	d, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("Failed to decode key")
	}

	if b.Type != privateKeyFileType {
		return nil, fmt.Errorf("File type did not match")
	}

	key, err = x509.ParseECPrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse key: %w", err)
	}
	return
}

// ReadPublicKeyFile reads a public key from the specified file
func ReadPublicKeyFile(keyFile string) (key *ecdsa.PublicKey, err error) {
	d, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("Failed to decode key")
	}

	if b.Type != publicKeyFileType {
		return nil, fmt.Errorf("File type did not match")
	}

	k, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse key: %w", err)
	}

	key, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("Key was of wrong type")
	}

	return
}
