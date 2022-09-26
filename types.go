package hotstuff

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// IDSet implements a set of replica IDs. It is used to show which replicas participated in some event.
type IDSet interface {
	// Add adds an ID to the set.
	Add(id ID)
	// Contains returns true if the set contains the ID.
	Contains(id ID) bool
	// ForEach calls f for each ID in the set.
	ForEach(f func(ID))
	// RangeWhile calls f for each ID in the set until f returns false.
	RangeWhile(f func(ID) bool)
	// Len returns the number of entries in the set.
	Len() int
}

// idSetMap implements IDSet using a map.
type idSetMap map[ID]struct{}

// NewIDSet returns a new IDSet using the default implementation.
func NewIDSet() IDSet {
	return make(idSetMap)
}

// Add adds an ID to the set.
func (s idSetMap) Add(id ID) {
	s[id] = struct{}{}
}

// Contains returns true if the set contains the given ID.
func (s idSetMap) Contains(id ID) bool {
	_, ok := s[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (s idSetMap) ForEach(f func(ID)) {
	for id := range s {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (s idSetMap) RangeWhile(f func(ID) bool) {
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
	set.ForEach(func(i ID) {
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

// ThresholdSignature is a signature that is only valid when it contains the signatures of a quorum of replicas.
//
// Deprecated: renamed to QuorumSignature
type ThresholdSignature = QuorumSignature

// PartialCert is a signed block hash.
type PartialCert struct {
	// shortcut to the signer of the signature
	signer    ID
	signature QuorumSignature
	blockHash Hash
}

// NewPartialCert returns a new partial certificate.
func NewPartialCert(signature QuorumSignature, blockHash Hash) PartialCert {
	var signer ID
	signature.Participants().RangeWhile(func(i ID) bool {
		signer = i
		return false
	})
	return PartialCert{signer, signature, blockHash}
}

// Signer returns the ID of the replica that created the certificate.
func (pc PartialCert) Signer() ID {
	return pc.signer
}

// Signature returns the signature.
func (pc PartialCert) Signature() QuorumSignature {
	return pc.signature
}

// BlockHash returns the hash of the block that was signed.
func (pc PartialCert) BlockHash() Hash {
	return pc.blockHash
}

// ToBytes returns a byte representation of the partial certificate.
func (pc PartialCert) ToBytes() []byte {
	return append(pc.blockHash[:], pc.signature.ToBytes()...)
}

// SyncInfo holds the highest known QC or TC.
// Generally, if highQC.View > highTC.View, there is no need to include highTC in the SyncInfo.
// However, if highQC.View < highTC.View, we should still include highQC.
// This can also hold an AggregateQC for Fast-Hotstuff.
type SyncInfo struct {
	qc    *QuorumCert
	tc    *TimeoutCert
	aggQC *AggregateQC
}

// NewSyncInfo returns a new SyncInfo struct.
func NewSyncInfo() SyncInfo {
	return SyncInfo{}
}

// WithQC returns a copy of the SyncInfo struct with the given QC.
func (si SyncInfo) WithQC(qc QuorumCert) SyncInfo {
	si.qc = new(QuorumCert)
	*si.qc = qc
	return si
}

// WithTC returns a copy of the SyncInfo struct with the given TC.
func (si SyncInfo) WithTC(tc TimeoutCert) SyncInfo {
	si.tc = new(TimeoutCert)
	*si.tc = tc
	return si
}

// WithAggQC returns a copy of the SyncInfo struct with the given AggregateQC.
func (si SyncInfo) WithAggQC(aggQC AggregateQC) SyncInfo {
	si.aggQC = new(AggregateQC)
	*si.aggQC = aggQC
	return si
}

// QC returns the quorum certificate, if present.
func (si SyncInfo) QC() (_ QuorumCert, _ bool) {
	if si.qc != nil {
		return *si.qc, true
	}
	return
}

// TC returns the timeout certificate, if present.
func (si SyncInfo) TC() (_ TimeoutCert, _ bool) {
	if si.tc != nil {
		return *si.tc, true
	}
	return
}

// AggQC returns the AggregateQC, if present.
func (si SyncInfo) AggQC() (_ AggregateQC, _ bool) {
	if si.aggQC != nil {
		return *si.aggQC, true
	}
	return
}

func (si SyncInfo) String() string {
	var sb strings.Builder
	sb.WriteString("{ ")
	if si.tc != nil {
		fmt.Fprintf(&sb, "%s ", si.tc)
	}
	if si.qc != nil {
		fmt.Fprintf(&sb, "%s ", si.qc)
	}
	if si.aggQC != nil {
		fmt.Fprintf(&sb, "%s ", si.aggQC)
	}
	sb.WriteRune('}')
	return sb.String()
}

// QuorumCert (QC) is a certificate for a Block created by a quorum of partial certificates.
type QuorumCert struct {
	signature QuorumSignature
	view      View
	hash      Hash
}

// NewQuorumCert creates a new quorum cert from the given values.
func NewQuorumCert(signature QuorumSignature, view View, hash Hash) QuorumCert {
	return QuorumCert{signature, view, hash}
}

// ToBytes returns a byte representation of the quorum certificate.
func (qc QuorumCert) ToBytes() []byte {
	b := qc.view.ToBytes()
	b = append(b, qc.hash[:]...)
	if qc.signature != nil {
		b = append(b, qc.signature.ToBytes()...)
	}
	return b
}

// Signature returns the threshold signature.
func (qc QuorumCert) Signature() QuorumSignature {
	return qc.signature
}

// BlockHash returns the hash of the block that was signed.
func (qc QuorumCert) BlockHash() Hash {
	return qc.hash
}

// View returns the view in which the QC was created.
func (qc QuorumCert) View() View {
	return qc.view
}

// Equals returns true if the other QC equals this QC.
func (qc QuorumCert) Equals(other QuorumCert) bool {
	if qc.view != other.view {
		return false
	}
	if qc.hash != other.hash {
		return false
	}
	if qc.signature == nil || other.signature == nil {
		return qc.signature == other.signature
	}
	return bytes.Equal(qc.signature.ToBytes(), other.signature.ToBytes())
}

func (qc QuorumCert) String() string {
	var sb strings.Builder
	if qc.signature != nil {
		_ = writeParticipants(&sb, qc.Signature().Participants())
	}
	return fmt.Sprintf("QC{ hash: %.6s, IDs: [ %s] }", qc.hash, &sb)
}

// TimeoutCert (TC) is a certificate created by a quorum of timeout messages.
type TimeoutCert struct {
	signature QuorumSignature
	view      View
}

// NewTimeoutCert returns a new timeout certificate.
func NewTimeoutCert(signature QuorumSignature, view View) TimeoutCert {
	return TimeoutCert{signature, view}
}

// ToBytes returns a byte representation of the timeout certificate.
func (tc TimeoutCert) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(tc.view))
	return append(viewBytes[:], tc.signature.ToBytes()...)
}

// Signature returns the threshold signature.
func (tc TimeoutCert) Signature() QuorumSignature {
	return tc.signature
}

// View returns the view in which the timeouts occurred.
func (tc TimeoutCert) View() View {
	return tc.view
}

func (tc TimeoutCert) String() string {
	var sb strings.Builder
	if tc.signature != nil {
		_ = writeParticipants(&sb, tc.Signature().Participants())
	}
	return fmt.Sprintf("TC{ view: %d, IDs: [ %s] }", tc.view, &sb)
}

// AggregateQC is a set of QCs extracted from timeout messages and an aggregate signature of the timeout signatures.
//
// This is used by the Fast-HotStuff consensus protocol.
type AggregateQC struct {
	qcs  map[ID]QuorumCert
	sig  QuorumSignature
	view View
}

// NewAggregateQC returns a new AggregateQC from the QC map and the threshold signature.
func NewAggregateQC(qcs map[ID]QuorumCert, sig QuorumSignature, view View) AggregateQC {
	return AggregateQC{qcs, sig, view}
}

// QCs returns the quorum certificates in the AggregateQC.
func (aggQC AggregateQC) QCs() map[ID]QuorumCert {
	return aggQC.qcs
}

// Sig returns the threshold signature in the AggregateQC.
func (aggQC AggregateQC) Sig() QuorumSignature {
	return aggQC.sig
}

// View returns the view in which the AggregateQC was created.
func (aggQC AggregateQC) View() View {
	return aggQC.view
}

func (aggQC AggregateQC) String() string {
	var sb strings.Builder
	if aggQC.sig != nil {
		_ = writeParticipants(&sb, aggQC.sig.Participants())
	}
	return fmt.Sprintf("AggQC{ view: %d, IDs: [ %s] }", aggQC.view, &sb)
}

func writeParticipants(wr io.Writer, participants IDSet) (err error) {
	participants.RangeWhile(func(id ID) bool {
		_, err = fmt.Fprintf(wr, "%d ", id)
		return err == nil
	})
	return err
}
