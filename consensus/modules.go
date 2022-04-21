package consensus

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

// Modules contains the modules that together implement the HotStuff protocol.
type Modules struct {
	// we embed a modules.Modules object so that we can use those modules too.
	*modules.Modules

	privateKey    PrivateKey
	opts          Options
	votingMachine *VotingMachine

	acceptor       Acceptor
	blockChain     BlockChain
	commandQueue   CommandQueue
	config         Configuration
	consensus      Consensus
	executor       ExecutorExt
	leaderRotation LeaderRotation
	crypto         Crypto
	synchronizer   Synchronizer
	forkHandler    ForkHandlerExt
}

// Run starts both event loops using the provided context and returns when both event loops have exited.
func (mods *Modules) Run(ctx context.Context) {
	mods.EventLoop().Run(ctx)
}

// PrivateKey returns the private key.
func (mods *Modules) PrivateKey() PrivateKey {
	return mods.privateKey
}

// Options returns the current configuration settings.
func (mods *Modules) Options() *Options {
	return &mods.opts
}

// Acceptor returns the acceptor.
func (mods *Modules) Acceptor() Acceptor {
	return mods.acceptor
}

// BlockChain returns the block chain.
func (mods *Modules) BlockChain() BlockChain {
	return mods.blockChain
}

// CommandQueue returns the command queue.
func (mods *Modules) CommandQueue() CommandQueue {
	return mods.commandQueue
}

// Configuration returns the configuration of replicas.
func (mods *Modules) Configuration() Configuration {
	return mods.config
}

// Consensus returns the consensus implementation.
func (mods *Modules) Consensus() Consensus {
	return mods.consensus
}

// Executor returns the executor.
func (mods *Modules) Executor() ExecutorExt {
	return mods.executor
}

// LeaderRotation returns the leader rotation implementation.
func (mods *Modules) LeaderRotation() LeaderRotation {
	return mods.leaderRotation
}

// Crypto returns the cryptography implementation.
func (mods *Modules) Crypto() Crypto {
	return mods.crypto
}

// Synchronizer returns the view synchronizer implementation.
func (mods *Modules) Synchronizer() Synchronizer {
	return mods.synchronizer
}

// ForkHandler returns the module responsible for handling forked blocks.
func (mods *Modules) ForkHandler() ForkHandlerExt {
	return mods.forkHandler
}

// Builder is a helper for constructing a HotStuff instance.
type Builder struct {
	baseBuilder modules.Builder
	mods        *Modules
	cfg         OptionsBuilder
	modules     []Module
}

// NewBuilder creates a new Builder.
func NewBuilder(id hotstuff.ID, privateKey PrivateKey) Builder {
	bl := Builder{
		baseBuilder: modules.NewBuilder(id),
		mods: &Modules{
			privateKey:    privateKey,
			votingMachine: NewVotingMachine(),
		},
	}
	// using a pointer here will allow settings to be readable within InitConsensusModule
	bl.cfg.opts = &bl.mods.opts
	// some of the default modules need to be registered
	bl.Register(bl.mods.votingMachine)
	return bl
}

// Register adds modules to the HotStuff object and initializes them.
// Modules are assigned to fields based on the interface they implement.
// If only the Module interface is implemented, the InitModule function will be called, but
// the HotStuff object will not save a reference to the module.
// Register will overwrite existing modules if the same type is registered twice.
func (b *Builder) Register(mods ...interface{}) { //nolint:gocyclo
	for _, module := range mods {
		b.baseBuilder.Register(module)
		if m, ok := module.(Acceptor); ok {
			b.mods.acceptor = m
		}
		if m, ok := module.(BlockChain); ok {
			b.mods.blockChain = m
		}
		if m, ok := module.(CommandQueue); ok {
			b.mods.commandQueue = m
		}
		if m, ok := module.(Configuration); ok {
			b.mods.config = m
		}
		if m, ok := module.(Consensus); ok {
			b.mods.consensus = m
		}
		if m, ok := module.(ExecutorExt); ok {
			b.mods.executor = m
		}
		if m, ok := module.(Executor); ok {
			b.mods.executor = executorWrapper{m}
		}
		if m, ok := module.(LeaderRotation); ok {
			b.mods.leaderRotation = m
		}
		if m, ok := module.(Crypto); ok {
			b.mods.crypto = m
		}
		if m, ok := module.(Synchronizer); ok {
			b.mods.synchronizer = m
		}
		if m, ok := module.(ForkHandlerExt); ok {
			b.mods.forkHandler = m
		}
		if m, ok := module.(ForkHandler); ok {
			b.mods.forkHandler = forkHandlerWrapper{m}
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// OptionsBuilder returns a pointer to the options builder.
// This can be used to configure runtime options.
func (b *Builder) OptionsBuilder() *OptionsBuilder {
	return &b.cfg
}

// Build initializes all modules and returns the HotStuff object.
func (b *Builder) Build() *Modules {
	b.mods.Modules = b.baseBuilder.Build()
	for _, module := range b.modules {
		module.InitConsensusModule(b.mods, &b.cfg)
	}
	return b.mods
}

// Module interfaces

// Module is an interface that can be implemented by types that need access to other consensus modules.
type Module interface {
	// InitConsensusModule gives the module a reference to the Modules object.
	// It also allows the module to set module options using the OptionsBuilder.
	InitConsensusModule(mods *Modules, _ *OptionsBuilder)
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

// Acceptor decides if a replica should accept a command.
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
	// Exec executes the command.
	Exec(cmd Command)
}

// ExecutorExt is responsible for executing the commands that are committed by the consensus protocol.
//
// This interface is similar to the Executor interface, except it takes a block as an argument, instead of a command,
// making it more flexible than the alternative interface.
type ExecutorExt interface {
	// Exec executes the command in the block.
	Exec(block *Block)
}

// ForkHandler handles commands that do not get committed due to a forked blockchain.
//
// TODO: think of a better name/interface
type ForkHandler interface {
	// Fork handles the command from a forked block.
	Fork(cmd Command)
}

// ForkHandlerExt handles blocks that do not get committed due to a fork of the blockchain.
//
// This interface is similar to the ForkHandler interface, except it takes a block as an argument, instead of a command.
type ForkHandlerExt interface {
	// Fork handles the forked block.
	Fork(block *Block)
}

// CryptoImpl implements only the cryptographic primitives that are needed for HotStuff.
// This interface is implemented by the ecdsa and bls12 packages.
type CryptoImpl interface {
	// Sign signs a hash.
	Sign(hash Hash) (sig Signature, err error)
	// Verify verifies a signature given a hash.
	Verify(sig Signature, hash Hash) bool
	// VerifyAggregateSignature verifies an aggregated signature.
	// It does not check whether the aggregated signature contains a quorum of signatures.
	VerifyAggregateSignature(agg ThresholdSignature, hash Hash) bool
	// CreateThresholdSignature creates a threshold signature from the given partial signatures.
	CreateThresholdSignature(partialSignatures []Signature, hash Hash) (ThresholdSignature, error)
	// CreateThresholdSignatureForMessageSet creates a threshold signature where each partial signature has signed a
	// different message hash.
	CreateThresholdSignatureForMessageSet(partialSignatures []Signature, hashes map[hotstuff.ID]Hash) (ThresholdSignature, error)
	// VerifyThresholdSignature verifies a threshold signature.
	VerifyThresholdSignature(signature ThresholdSignature, hash Hash) bool
	// VerifyThresholdSignatureForMessageSet verifies a threshold signature against a set of message hashes.
	VerifyThresholdSignatureForMessageSet(signature ThresholdSignature, hashes map[hotstuff.ID]Hash) bool
	// Combine combines multiple signatures into a single threshold signature.
	// Arguments can be singular signatures or threshold signatures.
	//
	// As opposed to the CreateThresholdSignature methods,
	// this method does not check whether the resulting
	// signature meets the quorum size.
	Combine(signatures ...interface{}) ThresholdSignature
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

	// Prunes blocks from the in-memory tree up to the specified height.
	// Returns a set of forked blocks (blocks that were on a different branch, and thus not committed).
	PruneToHeight(height View) (forkedBlocks []*Block)
}

//go:generate mockgen -destination=../internal/mocks/replica_mock.go -package=mocks . Replica

// Replica represents a remote replica participating in the consensus protocol.
// The methods Vote, NewView, and Deliver must send the respective arguments to the remote replica.
type Replica interface {
	// ID returns the replica's id.
	ID() hotstuff.ID
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
	Replicas() map[hotstuff.ID]Replica
	// Replica returns a replica if present in the configuration.
	Replica(hotstuff.ID) (replica Replica, ok bool)
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
	// CommittedBlock returns the most recently committed block.
	CommittedBlock() *Block
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(View) hotstuff.ID
}

//go:generate mockgen -destination=../internal/mocks/synchronizer_mock.go -package=mocks . Synchronizer

// Synchronizer synchronizes replicas to the same view.
type Synchronizer interface {
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(SyncInfo)
	// UpdateHighQC attempts to update HighQC using the given QC.
	UpdateHighQC(QuorumCert)
	// View returns the current view.
	View() View
	// ViewContext returns a context that is cancelled at the end of the view.
	ViewContext() context.Context
	// HighQC returns the highest known QC.
	HighQC() QuorumCert
	// LeafBlock returns the current leaf block.
	LeafBlock() *Block
	// Start starts the synchronizer with the given context.
	Start(context.Context)
}

type executorWrapper struct {
	executor Executor
}

func (ew executorWrapper) Exec(block *Block) {
	ew.executor.Exec(block.cmd)
}

type forkHandlerWrapper struct {
	forkHandler ForkHandler
}

func (fhw forkHandlerWrapper) Fork(block *Block) {
	fhw.forkHandler.Fork(block.cmd)
}
