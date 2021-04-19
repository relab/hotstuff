// Package hotstuff defines the core types and interfaces that implement the HotStuff protocol.
// These interfaces allow us to split the implementations into different modules,
// and each module can have multiple can have multiple implementations.
package hotstuff

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/relab/hotstuff/internal/logging"
)

// Basic types:

// ID uniquely identifies a replica
type ID uint32

// ToBytes returns the ID as bytes.
func (id ID) ToBytes() []byte {
	var idBytes [4]byte
	binary.LittleEndian.PutUint32(idBytes[:], uint32(id))
	return idBytes[:]
}

// IDSet implements a set of replica IDs. It is used to show which replicas participated in some event.
type IDSet interface {
	// Add adds an ID to the set.
	Add(id ID)
	// Contains returns true if the set contains the ID.
	Contains(id ID) bool
	// ForEach calls f for each ID in the set.
	ForEach(f func(ID))
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

// View is a number that uniquely identifies a view.
type View uint64

// ToBytes returns the view as bytes.
func (v View) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(v))
	return viewBytes[:]
}

// ToHash converts the view to a Hash type. It does not actually hash the view.
func (v View) ToHash() Hash {
	h := Hash{}
	binary.LittleEndian.PutUint64(h[:8], uint64(v))
	return h
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

// Signature is a cryptographic signature of a block.
type Signature interface {
	ToBytes
	// Signer returns the ID of the replica that created the signature.
	Signer() ID
}

// ThresholdSignature is a signature that is only valid when it contains the signatures of a quorum of replicas.
type ThresholdSignature interface {
	ToBytes
	// Participants returns the IDs of replicas who participated in the threshold signature.
	Participants() IDSet
}

// PartialCert is a signed block hash.
type PartialCert struct {
	signature Signature
	blockHash Hash
}

// NewPartialCert returns a new partial certificate.
func NewPartialCert(signature Signature, blockHash Hash) PartialCert {
	return PartialCert{signature, blockHash}
}

// Signature returns the signature.
func (pc PartialCert) Signature() Signature {
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
	var cert interface{}
	if si.qc != nil {
		cert = si.qc
	} else if si.tc != nil {
		cert = si.tc
	}
	return fmt.Sprint(cert)
}

// QuorumCert (QC) is a certificate for a Block created by a quorum of partial certificates.
type QuorumCert struct {
	signature ThresholdSignature
	view      View
	hash      Hash
}

// NewQuorumCert creates a new quorum cert from the given values.
func NewQuorumCert(signature ThresholdSignature, view View, hash Hash) QuorumCert {
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
func (qc QuorumCert) Signature() ThresholdSignature {
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

func (qc QuorumCert) String() string {
	var sb strings.Builder
	if qc.signature != nil {
		qc.signature.Participants().ForEach(func(id ID) {
			sb.WriteString(strconv.FormatUint(uint64(id), 10))
			sb.WriteByte(' ')
		})
	}
	return fmt.Sprintf("QC{ hash: %.6s, IDs: [ %s] }", qc.hash, &sb)
}

// TimeoutCert (TC) is a certificate created by a quorum of timeout messages.
type TimeoutCert struct {
	signature ThresholdSignature
	view      View
}

// NewTimeoutCert returns a new timeout certificate.
func NewTimeoutCert(signature ThresholdSignature, view View) TimeoutCert {
	return TimeoutCert{signature, view}
}

// ToBytes returns a byte representation of the timeout certificate.
func (tc TimeoutCert) ToBytes() []byte {
	var viewBytes [8]byte
	binary.LittleEndian.PutUint64(viewBytes[:], uint64(tc.view))
	return append(viewBytes[:], tc.signature.ToBytes()...)
}

// Signature returns the threshold signature.
func (tc TimeoutCert) Signature() ThresholdSignature {
	return tc.signature
}

// View returns the view in which the timeouts occurred.
func (tc TimeoutCert) View() View {
	return tc.view
}

func (tc TimeoutCert) String() string {
	var sb strings.Builder
	if tc.signature != nil {
		tc.signature.Participants().ForEach(func(id ID) {
			sb.WriteString(strconv.FormatUint(uint64(id), 10))
			sb.WriteByte(' ')
		})
	}
	return fmt.Sprintf("TC{ view: %d, IDs: [ %s] }", tc.view, &sb)
}

// AggregateQC is a set of QCs extracted from timeout messages and an aggregate signature of the timeout signatures.
type AggregateQC struct {
	qcs  map[ID]QuorumCert
	sig  ThresholdSignature
	view View
}

// NewAggregateQC returns a new AggregateQC from the QC map and the threshold signature.
func NewAggregateQC(qcs map[ID]QuorumCert, sig ThresholdSignature, view View) AggregateQC {
	return AggregateQC{qcs, sig, view}
}

// QCs returns the quorum certificates in the AggregateQC.
func (aggQC AggregateQC) QCs() map[ID]QuorumCert {
	return aggQC.qcs
}

// Sig returns the threshold signature in the AggregateQC.
func (aggQC AggregateQC) Sig() ThresholdSignature {
	return aggQC.sig
}

// View returns the view in which the AggregateQC was created.
func (aggQC AggregateQC) View() View {
	return aggQC.view
}

// Messages / Events

// Event is a common interface that should be implemented by all the message types below.
type Event interface{}

// EventProcessor processes events.
type EventProcessor interface {
	// ProcessEvent processes the given event.
	ProcessEvent(event Event)
}

// ProposeMsg is broadcast when a leader makes a proposal.
type ProposeMsg struct {
	ID          ID           // The ID of the replica who sent the message.
	Block       *Block       // The block that is proposed.
	AggregateQC *AggregateQC // Optional AggregateQC
}

// VoteMsg is sent to the leader by replicas voting on a proposal.
type VoteMsg struct {
	ID          ID          // the ID of the replica who sent the message.
	PartialCert PartialCert // The partial certificate.
	Deferred    bool
}

// TimeoutMsg is broadcast whenever a replica has a local timeout.
type TimeoutMsg struct {
	ID            ID        // The ID of the replica who sent the message.
	View          View      // The view that the replica wants to enter.
	ViewSignature Signature // A signature of the view
	MsgSignature  Signature // A signature of the view, QC.BlockHash, and the replica ID
	SyncInfo      SyncInfo  // The highest QC/TC known to the sender.
}

// Hash returns a hash of the timeout message.
func (timeout TimeoutMsg) Hash() Hash {
	var h Hash
	hash := sha256.New()
	hash.Write(timeout.View.ToBytes())
	if qc, ok := timeout.SyncInfo.QC(); ok {
		hash.Write(qc.hash[:])
	}
	hash.Write(timeout.ID.ToBytes())
	hash.Sum(h[:0])
	return h
}

func (timeout TimeoutMsg) String() string {
	return fmt.Sprintf("TimeoutMsg{ ID: %d, View: %d, SyncInfo: %v }", timeout.ID, timeout.View, timeout.SyncInfo)
}

// NewViewMsg is sent to the leader whenever a replica decides to advance to the next view.
// It contains the highest QC or TC known to the replica.
type NewViewMsg struct {
	ID       ID       // The ID of the replica who sent the message.
	SyncInfo SyncInfo // The highest QC / TC.
}

// ExponentialTimeout describes a timeout of the form Base * ExponentBase ^ Power, where Power <= MaxExponent.
type ExponentialTimeout struct {
	Base         time.Duration
	ExponentBase time.Duration
	MaxExponent  uint
}

// HotStuff contains the modules that together implement the HotStuff protocol.
type HotStuff struct {
	// data

	id         ID
	privateKey PrivateKey
	logger     logging.Logger
	cfg        Config
	eventLoop  *EventLoop

	// modules

	acceptor         Acceptor
	blockChain       BlockChain
	commandQueue     CommandQueue
	manager          Manager
	consensus        Consensus
	executor         Executor
	leaderRotation   LeaderRotation
	crypto           Crypto
	viewSynchronizer ViewSynchronizer
	votingMachine    VotingMachine
}

// ID returns the id.
func (hs *HotStuff) ID() ID {
	return hs.id
}

// PrivateKey returns the private key.
func (hs *HotStuff) PrivateKey() PrivateKey {
	return hs.privateKey
}

// Logger returns the logger.
func (hs *HotStuff) Logger() logging.Logger {
	return hs.logger
}

// Config returns the current configuration settings.
func (hs *HotStuff) Config() *Config {
	return &hs.cfg
}

// EventLoop returns the event loop.
func (hs *HotStuff) EventLoop() *EventLoop {
	return hs.eventLoop
}

// Acceptor returns the acceptor.
func (hs *HotStuff) Acceptor() Acceptor {
	return hs.acceptor
}

// BlockChain returns the block chain.
func (hs *HotStuff) BlockChain() BlockChain {
	return hs.blockChain
}

// CommandQueue returns the command queue.
func (hs *HotStuff) CommandQueue() CommandQueue {
	return hs.commandQueue
}

// Manager returns the manager used to communicate with the replicas.
func (hs *HotStuff) Manager() Manager {
	return hs.manager
}

// Consensus returns the consensus implementation.
func (hs *HotStuff) Consensus() Consensus {
	return hs.consensus
}

// Executor returns the executor.
func (hs *HotStuff) Executor() Executor {
	return hs.executor
}

// LeaderRotation returns the leader rotation implementation.
func (hs *HotStuff) LeaderRotation() LeaderRotation {
	return hs.leaderRotation
}

// Crypto returns the cryptography implementation.
func (hs *HotStuff) Crypto() Crypto {
	return hs.crypto
}

// ViewSynchronizer returns the view synchronizer implementation.
func (hs *HotStuff) ViewSynchronizer() ViewSynchronizer {
	return hs.viewSynchronizer
}

// VotingMachine returns the voting machine.
func (hs *HotStuff) VotingMachine() VotingMachine {
	return hs.votingMachine
}

// Builder is a helper for constructing a HotStuff instance.
type Builder struct {
	hs      *HotStuff
	cfg     ConfigBuilder
	modules []Module
}

// NewBuilder creates a new Builder.
func NewBuilder(id ID, privateKey PrivateKey) Builder {
	bl := Builder{hs: &HotStuff{
		id:         id,
		privateKey: privateKey,
		logger:     logging.New(""),
	}}
	bl.Register(NewEventLoop(100))
	return bl
}

// Register adds modules to the HotStuff object and initializes them.
// Modules are assigned to fields based on the interface they implement.
// If only the Module interface is implemented, the InitModule function will be called, but
// the HotStuff object will not save a reference to the module.
// Register will overwrite existing modules if the same type is registered twice.
func (b *Builder) Register(modules ...interface{}) {
	for _, module := range modules {
		if m, ok := module.(logging.Logger); ok {
			b.hs.logger = m
		}
		// allow overriding the event loop if a different buffer size is desired
		if m, ok := module.(*EventLoop); ok {
			b.hs.eventLoop = m
		}
		if m, ok := module.(Acceptor); ok {
			b.hs.acceptor = m
		}
		if m, ok := module.(BlockChain); ok {
			b.hs.blockChain = m
		}
		if m, ok := module.(CommandQueue); ok {
			b.hs.commandQueue = m
		}
		if m, ok := module.(Manager); ok {
			b.hs.manager = m
		}
		if m, ok := module.(Consensus); ok {
			b.hs.consensus = m
		}
		if m, ok := module.(Executor); ok {
			b.hs.executor = m
		}
		if m, ok := module.(LeaderRotation); ok {
			b.hs.leaderRotation = m
		}
		if m, ok := module.(Crypto); ok {
			b.hs.crypto = m
		}
		if m, ok := module.(ViewSynchronizer); ok {
			b.hs.viewSynchronizer = m
		}
		if m, ok := module.(VotingMachine); ok {
			b.hs.votingMachine = m
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all modules and returns the HotStuff object.
func (b *Builder) Build() *HotStuff {
	for _, module := range b.modules {
		module.InitModule(b.hs, &b.cfg)
	}
	b.hs.cfg = b.cfg.cfg
	return b.hs
}

// Module interfaces

// Module is an interface that can be implemented by types that need a reference to the HotStuff object.
type Module interface {
	// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
	// settings using the ConfigBuilder.
	InitModule(hs *HotStuff, _ *ConfigBuilder)
}

//go:generate mockgen -destination=internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed.
type CommandQueue interface {
	// Get returns the next command to be proposed.
	// It may run until the context is cancelled.
	// If no command is available, the 'ok' return value should be false.
	Get(ctx context.Context) (cmd Command, ok bool)
}

//go:generate mockgen -destination=internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor decides is a replica should accept a command.
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise.
	Accept(Command) bool
	// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
	// accepted in the future.
	Proposed(Command)
}

//go:generate mockgen -destination=internal/mocks/executor_mock.go -package=mocks . Executor

// Executor is responsible for executing the commands that are committed by the consensus protocol.
type Executor interface {
	// Exec executes the given command.
	Exec(Command)
}

// CryptoImpl implements only the cryptographic primitives that are needed for HotStuff.
// This interface is implemented by the ecdsa and bls12 packages.
type CryptoImpl interface {
	// Sign signs a hash.
	Sign(hash Hash) (sig Signature, err error)
	// Verify verifies a signature given a hash.
	Verify(sig Signature, hash Hash) bool
	// CreateThresholdSignature creates a threshold signature from the given partial signatures.
	CreateThresholdSignature(partialSignatures []Signature, hash Hash) (ThresholdSignature, error)
	// CreateThresholdSignatureForMessageSet creates a threshold signature where each partial signature has signed a
	// different message hash.
	CreateThresholdSignatureForMessageSet(partialSignatures []Signature, hashes map[ID]Hash) (ThresholdSignature, error)
	// VerifyThresholdSignature verifies a threshold signature.
	VerifyThresholdSignature(signature ThresholdSignature, hash Hash) bool
	// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
	VerifyThresholdSignatureForMessageSet(signature ThresholdSignature, hashes map[ID]Hash) bool
}

// Crypto implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type Crypto interface {
	CryptoImpl
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *Block) (cert PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *Block, signatures []PartialCert) (cert QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view View, timeouts []TimeoutMsg) (cert TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view View, timeouts []TimeoutMsg) (aggQC AggregateQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC AggregateQC) (ok bool, highQC QuorumCert)
}

// BlockChain is a datastructure that stores a chain of blocks.
// It is not required that a block is stored forever,
// but a block must be stored until at least one of its children have been committed.
type BlockChain interface {
	// Store stores a block in the blockchain.
	Store(*Block)

	// Get retrieves a block given its hash, attempting to fetching it from other replicas if necessary.
	Get(Hash) (*Block, bool)

	// LocalGet retrieves a block given its hash, without fetching it from other replicas.
	LocalGet(Hash) (*Block, bool)

	// Extends checks if the given block extends the branch of the target hash.
	Extends(block, target *Block) bool
}

//go:generate mockgen -destination=internal/mocks/replica_mock.go -package=mocks . Replica

// Replica represents a remote replica participating in the consensus protocol.
// The methods Vote, NewView, and Deliver must send the respective arguments to the remote replica.
type Replica interface {
	// ID returns the replica's id.
	ID() ID
	// PublicKey returns the replica's public key.
	PublicKey() PublicKey
	// Vote sends the partial certificate to the other replica.
	Vote(cert PartialCert)
	// NewView sends the quorum certificate to the other replica.
	NewView(SyncInfo)
}

//go:generate mockgen -destination=internal/mocks/manager_mock.go -package=mocks . Manager

// Manager holds information about the current configuration of replicas that participate in the protocol.
// It provides methods to send messages to the other replicas.
type Manager interface {
	// Replicas returns all of the replicas in the configuration.
	Replicas() map[ID]Replica
	// Replica returns a replica if present in the configuration.
	Replica(ID) (replica Replica, ok bool)
	// Len returns the number of replicas in the configuration.
	Len() int
	// QuorumSize returns the size of a quorum.
	QuorumSize() int
	// Propose sends the block to all replicas in the configuration.
	Propose(proposal ProposeMsg)
	// Timeout sends the timeout message to all replicas.
	Timeout(msg TimeoutMsg)
	// Fetch requests a block from all the replicas in the configuration.
	Fetch(ctx context.Context, hash Hash) (block *Block, ok bool)
}

//go:generate mockgen -destination=internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements a byzantine consensus protocol, such as HotStuff.
// It contains the protocol data for a single replica.
// The methods OnPropose, OnVote, OnNewView, and OnDeliver should be called upon receiving a corresponding message.
type Consensus interface {
	// StopVoting ensures that no voting happens in a view earlier than `view`.
	StopVoting(view View)
	// Propose starts a new proposal. The command is fetched from the command queue.
	Propose(cert SyncInfo)
	// OnPropose handles an incoming proposal.
	// A leader should call this method on itself.
	OnPropose(proposal ProposeMsg)
}

// VotingMachine handles incoming votes, and combines them into a Quorum Certificate when a quorum of votes is received.
type VotingMachine interface {
	// OnVote handles an incoming vote.
	OnVote(vote VoteMsg)
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(View) ID
}

//go:generate mockgen -destination=internal/mocks/synchronizer_mock.go -package=mocks . ViewSynchronizer

// ViewSynchronizer synchronizes replicas to the same view.
type ViewSynchronizer interface {
	// OnRemoteTimeout handles an incoming timeout from a remote replica.
	OnRemoteTimeout(TimeoutMsg)
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(SyncInfo)
	OnNewView(NewViewMsg)
	// View returns the current view.
	View() View
	// ViewContext returns a context that is cancelled at the end of the view.
	ViewContext() context.Context
	// UpdateHighQC updates the highest known QC.
	UpdateHighQC(QuorumCert)
	// HighQC returns the highest known QC.
	HighQC() QuorumCert
	// LeafBlock returns the current leaf block.
	LeafBlock() *Block
	// Start starts the synchronizer.
	Start()
	// Stop stops the synchronizer.
	Stop()
}
