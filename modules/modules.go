package modules

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/msg"
)

// ConsensusCore contains the modules that together implement consensus.
type ConsensusCore struct {
	*Core

	privateKey msg.PrivateKey
	opts       Options

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
	handel         Handel
}

// Run starts both event loops using the provided context and returns when both event loops have exited.
func (mods *ConsensusCore) Run(ctx context.Context) {
	mods.EventLoop().Run(ctx)
}

// PrivateKey returns the private key.
func (mods *ConsensusCore) PrivateKey() msg.PrivateKey {
	return mods.privateKey
}

// Options returns the current configuration settings.
func (mods *ConsensusCore) Options() *Options {
	return &mods.opts
}

// Acceptor returns the acceptor.
func (mods *ConsensusCore) Acceptor() Acceptor {
	return mods.acceptor
}

// BlockChain returns the block chain.
func (mods *ConsensusCore) BlockChain() BlockChain {
	return mods.blockChain
}

// CommandQueue returns the command queue.
func (mods *ConsensusCore) CommandQueue() CommandQueue {
	return mods.commandQueue
}

// Configuration returns the configuration of replicas.
func (mods *ConsensusCore) Configuration() Configuration {
	return mods.config
}

// Consensus returns the consensus implementation.
func (mods *ConsensusCore) Consensus() Consensus {
	return mods.consensus
}

// Executor returns the executor.
func (mods *ConsensusCore) Executor() ExecutorExt {
	return mods.executor
}

// LeaderRotation returns the leader rotation implementation.
func (mods *ConsensusCore) LeaderRotation() LeaderRotation {
	return mods.leaderRotation
}

// Crypto returns the cryptography implementation.
func (mods *ConsensusCore) Crypto() Crypto {
	return mods.crypto
}

// Synchronizer returns the view synchronizer implementation.
func (mods *ConsensusCore) Synchronizer() Synchronizer {
	return mods.synchronizer
}

// ForkHandler returns the module responsible for handling forked blocks.
func (mods *ConsensusCore) ForkHandler() ForkHandlerExt {
	return mods.forkHandler
}

// Handel returns the Handel implementation.
func (mods *ConsensusCore) Handel() Handel {
	return mods.handel
}

// ConsensusBuilder is a helper for constructing a ConsensusCore instance.
type ConsensusBuilder struct {
	baseBuilder CoreBuilder
	mods        *ConsensusCore
	cfg         OptionsBuilder
	modules     []ConsensusModule
}

// NewConsensusBuilder creates a new ConsensusBuilder.
func NewConsensusBuilder(id hotstuff.ID, privateKey msg.PrivateKey) ConsensusBuilder {
	bl := ConsensusBuilder{
		baseBuilder: NewCoreBuilder(id),
		mods: &ConsensusCore{
			privateKey: privateKey,
		},
	}
	// using a pointer here will allow settings to be readable within InitModule
	bl.cfg.opts = &bl.mods.opts
	bl.cfg.opts.connectionMetadata = make(map[string]string)
	return bl
}

// Register adds modules to the HotStuff object and initializes them.
// ConsensusCore are assigned to fields based on the interface they implement.
// If only the Module interface is implemented, the InitModule function will be called, but
// the HotStuff object will not save a reference to the module.
// Register will overwrite existing modules if the same type is registered twice.
func (b *ConsensusBuilder) Register(mods ...any) { //nolint:gocyclo
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
		if m, ok := module.(Handel); ok {
			b.mods.handel = m
		}
		if m, ok := module.(ConsensusModule); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// OptionsBuilder returns a pointer to the options builder.
// This can be used to configure runtime options.
func (b *ConsensusBuilder) OptionsBuilder() *OptionsBuilder {
	return &b.cfg
}

// Build initializes all modules and returns the HotStuff object.
func (b *ConsensusBuilder) Build() *ConsensusCore {
	b.mods.Core = b.baseBuilder.Build()
	for _, module := range b.modules {
		module.InitModule(b.mods, &b.cfg)
	}
	return b.mods
}

// Module interfaces

// ConsensusModule is an interface that can be implemented by types that need access to other consensus modules.
type ConsensusModule interface {
	// InitModule gives the module a reference to the ConsensusCore object.
	// It also allows the module to set module options using the OptionsBuilder.
	InitModule(mods *ConsensusCore, _ *OptionsBuilder)
}

//go:generate mockgen -destination=../internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed.
type CommandQueue interface {
	// Get returns the next command to be proposed.
	// It may run until the context is cancelled.
	// If no command is available, the 'ok' return value should be false.
	Get(ctx context.Context) (cmd msg.Command, ok bool)
}

//go:generate mockgen -destination=../internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor decides if a replica should accept a command.
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise.
	Accept(msg.Command) bool
	// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
	// accepted in the future.
	Proposed(msg.Command)
}

//go:generate mockgen -destination=../internal/mocks/executor_mock.go -package=mocks . Executor

// Executor is responsible for executing the commands that are committed by the consensus protocol.
type Executor interface {
	// Exec executes the command.
	Exec(cmd msg.Command)
}

// ExecutorExt is responsible for executing the commands that are committed by the consensus protocol.
//
// This interface is similar to the Executor interface, except it takes a block as an argument, instead of a command,
// making it more flexible than the alternative interface.
type ExecutorExt interface {
	// Exec executes the command in the block.
	Exec(block *msg.Block)
}

// ForkHandler handles commands that do not get committed due to a forked blockchain.
//
// TODO: think of a better name/interface
type ForkHandler interface {
	// Fork handles the command from a forked block.
	Fork(cmd msg.Command)
}

// ForkHandlerExt handles blocks that do not get committed due to a fork of the blockchain.
//
// This interface is similar to the ForkHandler interface, except it takes a block as an argument, instead of a command.
type ForkHandlerExt interface {
	// Fork handles the forked block.
	Fork(block *msg.Block)
}

// BlockChain is a datastructure that stores a chain of blocks.
// It is not required that a block is stored forever,
// but a block must be stored until at least one of its children have been committed.
type BlockChain interface {
	// Store stores a block in the blockchain.
	Store(*msg.Block)

	// Get retrieves a block given its hash, attempting to fetching it from other replicas if necessary.
	Get(msg.Hash) (*msg.Block, bool)

	// LocalGet retrieves a block given its hash, without fetching it from other replicas.
	LocalGet(msg.Hash) (*msg.Block, bool)

	// Extends checks if the given block extends the branch of the target hash.
	Extends(block, target *msg.Block) bool

	// Prunes blocks from the in-memory tree up to the specified height.
	// Returns a set of forked blocks (blocks that were on a different branch, and thus not committed).
	PruneToHeight(height msg.View) (forkedBlocks []*msg.Block)
}

//go:generate mockgen -destination=../internal/mocks/replica_mock.go -package=mocks . Replica

// Replica represents a remote replica participating in the consensus protocol.
// The methods Vote, NewView, and Deliver must send the respective arguments to the remote replica.
type Replica interface {
	// ID returns the replica's id.
	ID() hotstuff.ID
	// PublicKey returns the replica's public key.
	PublicKey() msg.PublicKey
	// Vote sends the partial certificate to the other replica.
	Vote(cert *msg.PartialCert)
	// NewView sends the quorum certificate to the other replica.
	NewView(*msg.SyncInfo)
	// Metadata returns the connection metadata sent by this replica.
	Metadata() map[string]string
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
	Propose(proposal *msg.Proposal)
	// Timeout sends the timeout message to all replicas.
	Timeout(msg *msg.TimeoutMsg)
	// Fetch requests a block from all the replicas in the configuration.
	Fetch(ctx context.Context, hash msg.Hash) (block *msg.Block, ok bool)
	// SubConfig returns a subconfiguration containing the replicas specified in the ids slice.
	SubConfig(ids []hotstuff.ID) (sub Configuration, err error)
}

//go:generate mockgen -destination=../internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements a byzantine consensus protocol, such as HotStuff.
// It contains the protocol data for a single replica.
// The methods OnPropose, OnVote, OnNewView, and OnDeliver should be called upon receiving a corresponding message.
type Consensus interface {
	// StopVoting ensures that no voting happens in a view earlier than `view`.
	StopVoting(view msg.View)
	// Propose starts a new proposal. The command is fetched from the command queue.
	Propose(cert *msg.SyncInfo)
	// CommittedBlock returns the most recently committed block.
	CommittedBlock() *msg.Block
	// ChainLength returns the number of blocks that need to be chained together in order to commit.
	ChainLength() int
}

// LeaderRotation implements a leader rotation scheme.
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view.
	GetLeader(msg.View) hotstuff.ID
}

//go:generate mockgen -destination=../internal/mocks/synchronizer_mock.go -package=mocks . Synchronizer

// Synchronizer synchronizes replicas to the same view.
type Synchronizer interface {
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(*msg.SyncInfo)
	// View returns the current view.
	View() msg.View
	// ViewContext returns a context that is cancelled at the end of the view.
	ViewContext() context.Context
	// HighQC returns the highest known QC.
	HighQC() *msg.QuorumCert
	// LeafBlock returns the current leaf block.
	LeafBlock() *msg.Block
	// Start starts the synchronizer with the given context.
	Start(context.Context)
}

// Handel is an implementation of the Handel signature aggregation protocol.
type Handel interface {
	// Begin commissions the aggregation of a new signature.
	Begin(s *msg.PartialCert)
}

type executorWrapper struct {
	executor Executor
}

func (ew executorWrapper) Exec(block *msg.Block) {
	ew.executor.Exec(block.Cmd())
}

type forkHandlerWrapper struct {
	forkHandler ForkHandler
}

func (fhw forkHandlerWrapper) Fork(block *msg.Block) {
	fhw.forkHandler.Fork(block.Cmd())
}
