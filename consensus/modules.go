package consensus

import (
	"context"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/logging"
)

// Modules contains the modules that together implement the HotStuff protocol.
type Modules struct {
	// data

	id            ID
	privateKey    PrivateKey
	logger        logging.Logger
	opts          Options
	eventLoop     *eventloop.EventLoop
	dataEventLoop *eventloop.EventLoop
	votingMachine *VotingMachine
	dataLogger    DataLogger

	// modules

	acceptor       Acceptor
	blockChain     BlockChain
	commandQueue   CommandQueue
	config         Configuration
	consensus      Consensus
	executor       Executor
	leaderRotation LeaderRotation
	crypto         Crypto
	synchronizer   Synchronizer
}

// Run starts both event loops using the provided context and returns when both event loops have exited.
func (mod *Modules) Run(ctx context.Context) {
	mainDone := make(chan struct{})
	go func() {
		mod.eventLoop.Run(ctx)
		close(mainDone)
	}()

	secondaryDone := make(chan struct{})
	go func() {
		mod.dataEventLoop.Run(ctx)
		close(secondaryDone)
	}()

	<-mainDone
	<-secondaryDone
}

// ID returns the id.
func (mod *Modules) ID() ID {
	return mod.id
}

// PrivateKey returns the private key.
func (mod *Modules) PrivateKey() PrivateKey {
	return mod.privateKey
}

// Logger returns the logger.
func (mod *Modules) Logger() logging.Logger {
	return mod.logger
}

// Options returns the current configuration settings.
func (mod *Modules) Options() *Options {
	return &mod.opts
}

// EventLoop returns the main event loop.
//
// This event loop should be used by components that participate in the consensus protocol.
// Events related to data collection should instead use the secondary event loop,
// which is accessible using the DataEventLoop() method.
func (mod *Modules) EventLoop() *eventloop.EventLoop {
	return mod.eventLoop
}

// DataEventLoop returns the secondary event loop, which is used for collection of data.
//
// This event loop should be used for handling data collection related events.
func (mod *Modules) DataEventLoop() *eventloop.EventLoop {
	return mod.dataEventLoop
}

// DataLogger returns the data logger.
func (mod *Modules) DataLogger() DataLogger {
	if mod.dataLogger == nil {
		return NopLogger()
	}
	return mod.dataLogger
}

// Acceptor returns the acceptor.
func (mod *Modules) Acceptor() Acceptor {
	return mod.acceptor
}

// BlockChain returns the block chain.
func (mod *Modules) BlockChain() BlockChain {
	return mod.blockChain
}

// CommandQueue returns the command queue.
func (mod *Modules) CommandQueue() CommandQueue {
	return mod.commandQueue
}

// Configuration returns the configuration of replicas.
func (mod *Modules) Configuration() Configuration {
	return mod.config
}

// Consensus returns the consensus implementation.
func (mod *Modules) Consensus() Consensus {
	return mod.consensus
}

// Executor returns the executor.
func (mod *Modules) Executor() Executor {
	return mod.executor
}

// LeaderRotation returns the leader rotation implementation.
func (mod *Modules) LeaderRotation() LeaderRotation {
	return mod.leaderRotation
}

// Crypto returns the cryptography implementation.
func (mod *Modules) Crypto() Crypto {
	return mod.crypto
}

// Synchronizer returns the view synchronizer implementation.
func (mod *Modules) Synchronizer() Synchronizer {
	return mod.synchronizer
}

// Builder is a helper for constructing a HotStuff instance.
type Builder struct {
	mod     *Modules
	cfg     OptionsBuilder
	modules []Module
}

// NewBuilder creates a new Builder.
func NewBuilder(id ID, privateKey PrivateKey) Builder {
	bl := Builder{mod: &Modules{
		id:            id,
		privateKey:    privateKey,
		logger:        logging.New(""),
		votingMachine: NewVotingMachine(),
		eventLoop:     eventloop.New(100), // TODO: make this configurable
		dataEventLoop: eventloop.New(100),
	}}
	// some of the default modules need to be registered
	bl.Register(bl.mod.eventLoop, bl.mod.dataEventLoop, bl.mod.votingMachine)
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
			b.mod.logger = m
		}
		if m, ok := module.(DataLogger); ok {
			b.mod.dataLogger = m
		}
		if m, ok := module.(Acceptor); ok {
			b.mod.acceptor = m
		}
		if m, ok := module.(BlockChain); ok {
			b.mod.blockChain = m
		}
		if m, ok := module.(CommandQueue); ok {
			b.mod.commandQueue = m
		}
		if m, ok := module.(Configuration); ok {
			b.mod.config = m
		}
		if m, ok := module.(Consensus); ok {
			b.mod.consensus = m
		}
		if m, ok := module.(Executor); ok {
			b.mod.executor = m
		}
		if m, ok := module.(LeaderRotation); ok {
			b.mod.leaderRotation = m
		}
		if m, ok := module.(Crypto); ok {
			b.mod.crypto = m
		}
		if m, ok := module.(Synchronizer); ok {
			b.mod.synchronizer = m
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all modules and returns the HotStuff object.
func (b *Builder) Build() *Modules {
	for _, module := range b.modules {
		module.InitModule(b.mod, &b.cfg)
	}
	b.mod.opts = b.cfg.opts
	return b.mod
}

// Module interfaces

// Module is an interface that can be implemented by types that need a reference to the HotStuff object.
type Module interface {
	// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
	// settings using the ConfigBuilder.
	InitModule(mod *Modules, _ *OptionsBuilder)
}

//go:generate mockgen -destination=../internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed.
type CommandQueue interface {
	// Get returns the next command to be proposed.
	// It may run until the context is cancelled.
	// If no command is available, the 'ok' return value should be false.
	Get(ctx context.Context) (cmd Command, ok bool)
}

//go:generate mockgen -destination=../internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor decides is a replica should accept a command.
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise.
	Accept(Command) bool
	// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
	// accepted in the future.
	Proposed(Command)
}

//go:generate mockgen -destination=../internal/mocks/executor_mock.go -package=mocks . Executor

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

//go:generate mockgen -destination=../internal/mocks/replica_mock.go -package=mocks . Replica

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

//go:generate mockgen -destination=../internal/mocks/configuration_mock.go -package=mocks . Configuration

// Configuration holds information about the current configuration of replicas that participate in the protocol,
// It provides methods to send messages to the other replicas.
type Configuration interface {
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

//go:generate mockgen -destination=../internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements a byzantine consensus protocol, such as HotStuff.
// It contains the protocol data for a single replica.
// The methods OnPropose, OnVote, OnNewView, and OnDeliver should be called upon receiving a corresponding message.
type Consensus interface {
	// StopVoting ensures that no voting happens in a view earlier than `view`.
	StopVoting(view View)
	// Propose starts a new proposal. The command is fetched from the command queue.
	Propose(cert SyncInfo)
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(View) ID
}

//go:generate mockgen -destination=../internal/mocks/synchronizer_mock.go -package=mocks . Synchronizer

// Synchronizer synchronizes replicas to the same view.
type Synchronizer interface {
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(SyncInfo)
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
	// Start starts the synchronizer with the given context.
	Start(context.Context)
}
