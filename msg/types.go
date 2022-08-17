package msg

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/relab/hotstuff"
	"google.golang.org/protobuf/proto"
)

// IDSet implements a set of replica IDs. It is used to show which replicas participated in some event.
type IDSet interface {
	// Add adds an ID to the set.
	Add(id hotstuff.ID)
	// Contains returns true if the set contains the ID.
	Contains(id hotstuff.ID) bool
	// ForEach calls f for each ID in the set.
	ForEach(f func(hotstuff.ID))
	// RangeWhile calls f for each ID in the set until f returns false.
	RangeWhile(f func(hotstuff.ID) bool)
	// Len returns the number of entries in the set.
	Len() int
}

// idSetMap implements IDSet using a map.
type idSetMap map[hotstuff.ID]struct{}

// NewIDSet returns a new IDSet using the default implementation.
func NewIDSet() IDSet {
	return make(idSetMap)
}

// Add adds an ID to the set.
func (s idSetMap) Add(id hotstuff.ID) {
	s[id] = struct{}{}
}

// Contains returns true if the set contains the given ID.
func (s idSetMap) Contains(id hotstuff.ID) bool {
	_, ok := s[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (s idSetMap) ForEach(f func(hotstuff.ID)) {
	for id := range s {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (s idSetMap) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range s {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (s idSetMap) Len() int {
	return len(s)
}

func (s idSetMap) String() string {
	return IDSetToString(s)
}

// IDSetToString formats an IDSet as a string.
func IDSetToString(set IDSet) string {
	var sb strings.Builder
	sb.WriteString("[ ")
	set.ForEach(func(i hotstuff.ID) {
		sb.WriteString(strconv.Itoa(int(i)))
		sb.WriteString(" ")
	})
	sb.WriteString("]")
	return sb.String()
}

// View is a number that uniquely identifies a view.
type View uint64

// ToBytes returns the view as bytes.
func (v View) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(v))
	return viewBytes[:]
}

// Hash is a SHA256 hash
type Hash [32]byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h[:])
}

func (h Hash) ToBytes() []byte {

	hash := make([]byte, 0)
	for _, b := range h {
		hash = append(hash, b)
	}

	return hash
}

func ToHash(hash []byte) Hash {
	var h Hash
	copy(h[:], hash[:])
	return h
}

// Command is a client request to be executed by the consensus protocol.
//
// The string type is used because it is immutable and can hold arbitrary bytes of any length.
type Command string

// ToBytes is an object that can be converted into bytes for the purposes of hashing, etc.
type ToBytes interface {
	// ToBytes returns the object as bytes.
	ToBytes() []byte
}

// PublicKey is the public part of a replica's key pair.
type PublicKey = crypto.PublicKey

// PrivateKey is the private part of a replica's key pair.
type PrivateKey interface {
	// Public returns the public key associated with this private key.
	Public() PublicKey
}

// QuorumSignature is a signature that is only valid when it contains the signatures of a quorum of replicas.
type QuorumSignature interface {
	ToBytes
	// Participants returns the IDs of replicas who participated in the threshold signature.
	Participants() IDSet
}

// PartialCert is a signed block hash.
// type PartialCert struct {
// 	// shortcut to the signer of the signature
// 	signer    hotstuff.ID
// 	signature QuorumSignature
// 	blockHash Hash
// }

// // NewPartialCert returns a new partial certificate.
// func NewPartialCert(signature QuorumSignature, blockHash Hash) PartialCert {
// 	var signer hotstuff.ID
// 	signature.Participants().RangeWhile(func(i hotstuff.ID) bool {
// 		signer = i
// 		return false
// 	})
// 	return PartialCert{signer, signature, blockHash}
// }

// // Signer returns the ID of the replica that created the certificate.
// func (pc PartialCert) Signer() hotstuff.ID {
// 	return pc.signer
// }

// // Signature returns the signature.
// func (pc PartialCert) Signature() QuorumSignature {
// 	return pc.signature
// }

// // BlockHash returns the hash of the block that was signed.
// func (pc PartialCert) BlockHash() Hash {
// 	return pc.blockHash
// }

// // ToBytes returns a byte representation of the partial certificate.
// func (pc PartialCert) ToBytes() []byte {
// 	return append(pc.blockHash[:], pc.signature.ToBytes()...)
// }

// SyncInfo holds the highest known QC or TC.
// Generally, if highQC.View > highTC.View, there is no need to include highTC in the SyncInfo.
// However, if highQC.View < highTC.View, we should still include highQC.
// This can also hold an AggregateQC for Fast-Hotstuff.
// type SyncInfo struct {
// 	QCert    *QuorumCert
// 	TCert    *TimeoutCert
// 	AggQCert *AggregateQC
// }

// NewSyncInfo returns a new SyncInfo struct.
func NewSyncInfo() *SyncInfo {
	return &SyncInfo{}
}

// WithQC returns a copy of the SyncInfo struct with the given QC.
func (si *SyncInfo) WithQC(qc *QuorumCert) *SyncInfo {
	//si.QCert = new(QuorumCert)
	si.QCert = qc
	return si
}

// WithTC returns a copy of the SyncInfo struct with the given TC.
func (si *SyncInfo) WithTC(tc *TimeoutCert) *SyncInfo {
	si.TCert = new(TimeoutCert)
	si.TCert = tc
	return si
}

// WithAggQC returns a copy of the SyncInfo struct with the given AggregateQC.
func (si *SyncInfo) WithAggQC(aggQC *AggQC) *SyncInfo {
	si.AggQCert = new(AggQC)
	si.AggQCert = aggQC
	return si
}

// QC returns the quorum certificate, if present.
func (si *SyncInfo) QC() (_ *QuorumCert, _ bool) {
	if si.QCert != nil {
		return si.QCert, true
	}
	return nil, false
}

// TC returns the timeout certificate, if present.
func (si *SyncInfo) TC() (_ *TimeoutCert, _ bool) {
	if si.TCert != nil {
		return si.TCert, true
	}
	return nil, false
}

// AggQC returns the AggregateQC, if present.
func (si *SyncInfo) AggQC() (_ *AggQC, _ bool) {
	if si.AggQCert != nil {
		return si.AggQCert, true
	}
	return nil, false
}

// func (si SyncInfo) SString() string {
// 	var sb strings.Builder
// 	sb.WriteString("{ ")
// 	if si.TCert != nil {
// 		fmt.Fprintf(&sb, "%s ", si.TCert.TCString())
// 	}
// 	if si.QCert != nil {
// 		fmt.Fprintf(&sb, "%s ", si.QCert.QCString())
// 	}
// 	if si.AggQCert != nil {
// 		fmt.Fprintf(&sb, "%s ", si.AggQCert.AQCString())
// 	}
// 	sb.WriteRune('}')
// 	return sb.String()
// }

// QuorumCert (QC) is a certificate for a Block created by a quorum of partial certificates.
// type QuorumCert struct {
// 	Sig  QuorumSignature
// 	View View
// 	Hash Hash
// }

// NewQuorumCert creates a new quorum cert from the given values.
func NewQuorumCert(signature *ThresholdSignature, view View, hash Hash) *QuorumCert {
	return &QuorumCert{
		Sig:  signature,
		View: uint64(view),
		Hash: hash.ToBytes(),
	}
}

// ToBytes returns a byte representation of the quorum certificate.
func (qc *QuorumCert) ToBytes() []byte {
	// b := qc.View.ToBytes()
	// b = append(b, qc.Hash[:]...)
	// if qc.Sig != nil {
	// 	b = append(b, qc.Sig.ToBytes()...)
	// }
	// return b
	if qc == nil {
		return nil
	}
	y, err := proto.Marshal(qc)
	if err != nil {
		return nil
	}
	return y
}

// Signature returns the threshold signature.
func (qc *QuorumCert) Signature() *ThresholdSignature {
	return qc.Sig
}

// BlockHash returns the hash of the block that was signed.
func (qc *QuorumCert) BlockHash() Hash {
	return ToHash(qc.Hash)
}

// QCView returns the view in which the QC was created.
func (qc *QuorumCert) QCView() View {
	return View(qc.View)
}

// Equals returns true if the other QC equals this QC.
func (qc *QuorumCert) Equals(other *QuorumCert) bool {
	if qc.View != other.View {
		return false
	}
	if !bytes.Equal(qc.Hash, other.Hash) {
		return false
	}
	if qc.Sig == nil || other.Sig == nil {
		return qc.Sig == other.Sig
	}
	return bytes.Equal(qc.Sig.ToBytes(), other.Sig.ToBytes())
}

func (qc *QuorumCert) QCString() string {
	var sb strings.Builder
	if qc.Sig != nil {
		_ = writeParticipants(&sb, qc.Signature().Participants())
	}
	return fmt.Sprintf("QC{ hash: %.6s, IDs: [ %s] }", qc.Hash, &sb)
}

// TimeoutCert (TC) is a certificate created by a quorum of timeout messages.
// type TimeoutCert struct {
// 	Sig  QuorumSignature
// 	View View
// }

// NewTimeoutCert returns a new timeout certificate.
func NewTimeoutCert(signature *ThresholdSignature, view View) *TimeoutCert {
	return &TimeoutCert{Sig: signature, View: uint64(view)}
}

// ToBytes returns a byte representation of the timeout certificate.
func (tc *TimeoutCert) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(tc.View))
	return append(viewBytes[:], tc.Sig.ToBytes()...)
}

// Signature returns the threshold signature.
func (tc *TimeoutCert) Signature() QuorumSignature {
	return tc.Sig
}

// TCView returns the view in which the timeouts occurred.
func (tc *TimeoutCert) TCView() View {
	return View(tc.View)
}

func (tc *TimeoutCert) TCString() string {
	var sb strings.Builder
	if tc.Sig != nil {
		_ = writeParticipants(&sb, tc.Signature().Participants())
	}
	return fmt.Sprintf("TC{ view: %d, IDs: [ %s] }", tc.View, &sb)
}

// AggregateQC is a set of QCs extracted from timeout messages and an aggregate signature of the timeout signatures.
//
// This is used by the Fast-HotStuff consensus protocol.
// type AggregateQC struct {
// 	QCs  map[hotstuff.ID]QuorumCert
// 	Sig  QuorumSignature
// 	View View
// }

func NewAggregateQC(qcs map[hotstuff.ID]*QuorumCert, sig *ThresholdSignature, view View) *AggQC {
	pqcs := make(map[uint32]*QuorumCert)
	for id, qc := range qcs {
		pqcs[uint32(id)] = qc
	}
	return &AggQC{
		QCs:  pqcs,
		Sig:  sig,
		View: uint64(view),
	}
}

// // NewAggregateQC returns a new AggregateQC from the QC map and the threshold signature.
// func NewAggregateQC(qcs map[hotstuff.ID]QuorumCert, sig QuorumSignature, view View) *AggQC {
// 	pqcs := make(map[uint32]*QuorumCert)
// 	for id, qc := range qcs {
// 		pqcs[uint32(id)] = &qc
// 	}
// 	return &AggQC{
// 		QCs:  pqcs,
// 		Sig:  sig,
// 		View: uint64(view),
// 	}
// }

// // QCerts returns the quorum certificates in the AggregateQC.
// func (aggQC *AggQC) QCerts() map[hotstuff.ID]*QuorumCert {
// 	return aggQC.QCs
// }

// // Signature returns the threshold signature in the AggregateQC.
// func (aggQC AggregateQC) Signature() QuorumSignature {
// 	return aggQC.Sig
// }

// // AQCView returns the view in which the AggregateQC was created.
// func (aggQC AggregateQC) AQCView() View {
// 	return aggQC.View
// }

// func (aggQC AggregateQC) AQCString() string {
// 	var sb strings.Builder
// 	if aggQC.Sig != nil {
// 		_ = writeParticipants(&sb, aggQC.Sig.Participants())
// 	}
// 	return fmt.Sprintf("AggQC{ view: %d, IDs: [ %s] }", aggQC.View, &sb)
// }

func writeParticipants(wr io.Writer, participants IDSet) (err error) {
	participants.RangeWhile(func(id hotstuff.ID) bool {
		_, err = fmt.Fprintf(wr, "%d ", id)
		return err == nil
	})
	return err
}

func (sig *ECDSASignature) ToBytes() []byte {
	var b []byte
	b = append(b, sig.GetR()...)
	b = append(b, sig.GetS()...)
	return b
}

func (x *ThresholdSignature) Participants() IDSet {

	idSet := NewIDSet()
	if x == nil {
		return idSet
	}
	switch x.AggSig.(type) {
	case *ThresholdSignature_ECDSASigs:
		ecdsaTS := x.AggSig.(*ThresholdSignature_ECDSASigs)
		for _, sig := range ecdsaTS.ECDSASigs.Sigs {
			idSet.Add(hotstuff.ID(sig.Signer))
		}
	}
	return idSet
}

func (x *ThresholdSignature) ToBytes() []byte {
	if x == nil {
		return nil
	}
	y, err := proto.Marshal(x)
	if err != nil {
		return nil
	}
	return y
}

func (sig *Signature) CreateThresholdSignature() *ThresholdSignature {
	switch sig.Sig.(type) {

	case *Signature_ECDSASig:
		signatures := make([]*ECDSASignature, 0)
		signature := &ECDSASignature{
			Signer: sig.ID,
			R:      sig.GetECDSASig().GetR(),
			S:      sig.GetECDSASig().GetS(),
		}
		signatures = append(signatures, signature)
		return &ThresholdSignature{
			AggSig: &ThresholdSignature_ECDSASigs{
				ECDSASigs: &ECDSAThresholdSignature{
					Sigs: signatures,
				},
			},
		}
	}
	return nil
}

func ViewToBytes(v uint64) []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], v)
	return viewBytes[:]
}
