package core

import (
	"context"

	"github.com/relab/hotstuff"
)

// Component interfaces

//go:generate mockgen -destination=../internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed.
type CommandQueue interface {
	// Get returns the next command to be proposed.
	// It may run until the context is canceled.
	// If no command is available, the 'ok' return value should be false.
	Get(ctx context.Context) (cmd hotstuff.Command, ok bool)
}

//go:generate mockgen -destination=../internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor decides if a replica should accept a command.
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise.
	Accept(hotstuff.Command) bool
	// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
	// accepted in the future.
	Proposed(hotstuff.Command)
}

//go:generate mockgen -destination=../internal/mocks/executor_mock.go -package=mocks . Executor

// Executor is responsible for executing the commands that are committed by the consensus protocol.
type Executor interface {
	// Exec executes the command.
	Exec(cmd hotstuff.Command)
}

// ExecutorExt is responsible for executing the commands that are committed by the consensus protocol.
//
// This interface is similar to the Executor interface, except it takes a block as an argument, instead of a command,
// making it more flexible than the alternative interface.
type ExecutorExt interface {
	// Exec executes the command in the block.
	Exec(block *hotstuff.Block)
}

//go:generate mockgen -destination=../internal/mocks/forkhandler_mock.go -package=mocks . ForkHandler

// ForkHandler handles commands that do not get committed due to a forked blockchain.
//
// TODO: think of a better name/interface
type ForkHandler interface {
	// Fork handles the command from a forked block.
	Fork(cmd hotstuff.Command)
}

// ForkHandlerExt handles blocks that do not get committed due to a fork of the blockchain.
//
// This interface is similar to the ForkHandler interface, except it takes a block as an argument, instead of a command.
type ForkHandlerExt interface {
	// Fork handles the forked block.
	Fork(block *hotstuff.Block)
}

// BlockChain is a datastructure that stores a chain of blocks.
// It is not required that a block is stored forever,
// but a block must be stored until at least one of its children have been committed.
type BlockChain interface {
	// Store stores a block in the blockchain.
	Store(*hotstuff.Block)

	// Get retrieves a block given its hash, attempting to fetching it from other replicas if necessary.
	Get(hash hotstuff.Hash) (*hotstuff.Block, bool)

	// LocalGet retrieves a block given its hash, without fetching it from other replicas.
	LocalGet(hotstuff.Hash) (*hotstuff.Block, bool)

	// Extends checks if the given block extends the branch of the target hash.
	Extends(block, target *hotstuff.Block) bool

	// Prunes blocks from the in-memory tree up to the specified height.
	// Returns a set of forked blocks (blocks that were on a different branch, and thus not committed).
	PruneToHeight(height hotstuff.View) (prunedBlocks map[hotstuff.View][]*hotstuff.Block)

	PruneHeight() hotstuff.View

	DeleteAtHeight(height hotstuff.View, blockHash hotstuff.Hash) error
}

//go:generate mockgen -destination=../internal/mocks/replica_mock.go -package=mocks . Replica

// Replica represents a remote replica participating in the consensus protocol.
// The methods Vote, NewView, and Deliver must send the respective arguments to the remote replica.
type Replica interface {
	// ID returns the replica's id.
	ID() hotstuff.ID
	// PublicKey returns the replica's public key.
	PublicKey() hotstuff.PublicKey
	// Vote sends the partial certificate to the other replica.
	Vote(cert hotstuff.PartialCert)
	// NewView sends the quorum certificate to the other replica.
	NewView(hotstuff.SyncInfo)
	// Metadata returns the connection metadata sent by this replica.
	Metadata() map[string]string
}

//go:generate mockgen -destination=../internal/mocks/configuration_mock.go -package=mocks . Configuration

// Configuration holds information about the current configuration of replicas that participate in the protocol,
type Configuration interface {
	// Replicas returns all of the replicas in the configuration.
	Replicas() map[hotstuff.ID]*hotstuff.ReplicaInfo
	// Replica returns a replica if present in the configuration.
	Replica(hotstuff.ID) (replica *hotstuff.ReplicaInfo, ok bool)
	// Len returns the number of replicas in the configuration.
	Len() int
	// QuorumSize returns the size of a quorum.
	QuorumSize() int
	// GetSubConfig returns a subconfiguration containing the replicas specified in the ids slice.
	// TODO: is this really needed?
	// GetSubConfig(ids []hotstuff.ID) (sub Configuration, err error)
}

//go:generate mockgen -destination=../internal/mocks/protocolinvoker_mock.go -package=mocks . ProtocolInvoker

// ProtocolInvoker provides methods to send messages to the other replicas through
// knowledge of the configuration of participant replicas in the protocol.
type ProtocolInvoker interface {
	// Propose sends the block to all replicas in the configuration.
	Propose(proposal hotstuff.ProposeMsg)
	// Timeout sends the timeout message to all replicas.
	Timeout(msg hotstuff.TimeoutMsg)
	// Fetch requests a block from all the replicas in the configuration.
	Fetch(ctx context.Context, hash hotstuff.Hash) (block *hotstuff.Block, ok bool)
	// ReplicaNode returns the internal replica with methods to invoke the protocol
	ReplicaNode(id hotstuff.ID) (Replica, bool)
}

//go:generate mockgen -destination=../internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements the general logic of a byzantine consensus protocol.
// It contains the protocol data for a single replica.
// The methods OnPropose, OnVote, OnNewView, and OnDeliver should be called upon receiving a corresponding message.
type Consensus interface {
	// StopVoting ensures that no voting happens in a view earlier than `view`.
	StopVoting(view hotstuff.View)
	// Propose starts a new proposal. The command is fetched from the command queue.
	Propose(view hotstuff.View, cert hotstuff.SyncInfo) (syncInfo hotstuff.SyncInfo, advance bool)
}

//go:generate mockgen -destination=../internal/mocks/committer_mock.go -package=mocks . Committer

// Committer is a helper module which handles block commits and forks.
type Committer interface {
	// Stores the block before further execution.
	Commit(committedHeight hotstuff.View, block *hotstuff.Block)

	// CommittedBlock returns the most recently committed block.
	CommittedBlock() *hotstuff.Block
}

type VotingMachine interface {
	OnVote(vote hotstuff.VoteMsg)
}

//go:generate mockgen -destination=../internal/mocks/synchronizer_mock.go -package=mocks . Synchronizer

// Synchronizer synchronizes replicas to the same view.
type Synchronizer interface {
	// AdvanceView attempts to advance to the next view using the given QC.
	// qc must be either a regular quorum certificate, or a timeout certificate.
	AdvanceView(hotstuff.SyncInfo)
	// View returns the current view.
	View() hotstuff.View
	// HighQC returns the highest known QC.
	HighQC() hotstuff.QuorumCert
	// Start starts the synchronizer with the given context.
	Start(context.Context)
}

// CertAuth implements the methods required to create and verify signatures and certificates.
// This is a higher level interface that is implemented by the crypto package itself.
type CertAuth interface {
	// CreatePartialCert signs a single block and returns the partial certificate.
	CreatePartialCert(block *hotstuff.Block) (cert hotstuff.PartialCert, err error)
	// CreateQuorumCert creates a quorum certificate from a list of partial certificates.
	CreateQuorumCert(block *hotstuff.Block, signatures []hotstuff.PartialCert) (cert hotstuff.QuorumCert, err error)
	// CreateTimeoutCert creates a timeout certificate from a list of timeout messages.
	CreateTimeoutCert(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (cert hotstuff.TimeoutCert, err error)
	// CreateAggregateQC creates an AggregateQC from the given timeout messages.
	CreateAggregateQC(view hotstuff.View, timeouts []hotstuff.TimeoutMsg) (aggQC hotstuff.AggregateQC, err error)
	// VerifyPartialCert verifies a single partial certificate.
	VerifyPartialCert(cert hotstuff.PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate.
	VerifyQuorumCert(qc hotstuff.QuorumCert) bool
	// VerifyTimeoutCert verifies a timeout certificate.
	VerifyTimeoutCert(tc hotstuff.TimeoutCert) bool
	// VerifyAggregateQC verifies an AggregateQC.
	VerifyAggregateQC(aggQC hotstuff.AggregateQC) (highQC hotstuff.QuorumCert, ok bool)
}

// Handel is an implementation of the Handel signature aggregation protocol.
type Handel interface {
	// Begin commissions the aggregation of a new signature.
	Begin(s hotstuff.PartialCert)
}

// ExtendedExecutor turns the given Executor into an ExecutorExt.
func ExtendedExecutor(executor Executor) ExecutorExt {
	return executorWrapper{executor}
}

type executorWrapper struct {
	executor Executor
}

func (ew executorWrapper) InitModule(mods *Core) {
	if m, ok := ew.executor.(Module); ok {
		m.InitModule(mods)
	}
}

func (ew executorWrapper) Exec(block *hotstuff.Block) {
	ew.executor.Exec(block.Command())
}

// ExtendedForkHandler turns the given ForkHandler into a ForkHandlerExt.
func ExtendedForkHandler(forkHandler ForkHandler) ForkHandlerExt {
	return forkHandlerWrapper{forkHandler}
}

type forkHandlerWrapper struct {
	forkHandler ForkHandler
}

func (fhw forkHandlerWrapper) InitModule(mods *Core) {
	if m, ok := fhw.forkHandler.(Module); ok {
		m.InitModule(mods)
	}
}

func (fhw forkHandlerWrapper) Fork(block *hotstuff.Block) {
	fhw.forkHandler.Fork(block.Command())
}
