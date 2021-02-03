// Package hotstuff defines the core types and interfaces that implement the HotStuff protocol.
// These interfaces allow us to split the implementations into different modules,
// and each module can have multiple can have multiple implementations.
//
// The following diagram illustrates the relationships between these interfaces:
//
//                  OnDeliver()------------------------+
//                                                     |                 +--------------+
//                  OnPropose()---------------------+  |  +--Accept()--->|   Acceptor   |
//                                                  |  |  |              +--------------+
//                  OnVote()---------------------+  |  |  |
//                                               |  |  |  |              +--------------+
//                  OnNewView()---------------+  |  |  |  |  +--Exec()-->|   Executor   |
//                                            |  |  |  |  |  |           +--------------+
//                                            v  v  v  v  |  |
//  +--------------+                       +-------------------------+                     +----------------------+
//  |              |                       |                         |<-----Propose()------|                      |
//  |              |<--------Sign()--------|                         |                     |                      |
//  |    Signer    |                       |                         |<-----NewView()------|                      |
//  |              |<--CreateQuorumCert()--|                         |                     |                      |
//  |              |                       |                         |------OnPropose()--->|   ViewSynchronizer   |
//  +--------------+                       |                         |                     |                      |
//                                         |        Consensus        |------OnNewView()--->|                      |
//  +--------------+                       |                         |                     |                      |
//  |              |                       |                         |------OnFinishQC()-->|                      |
//  |              |<--VerifyQuorumCert()--|                         |                     +----------------------+
//  |   Verifier   |                       |                         |                                 |
//  |              |<-VerifyPartialCert()--|                         |                                 |
//  |              |                       |                         |----------GetLeader()------------+
//  +--------------+                       +-------------------------+             |
//                                           |  |  |  |  |  |                      v
//  +-----------------+                      |  |  |  |  |  |             +--------|-------+
//  |                 |<----Propose()--------+  |  |  |  |  |             | LeaderRotation |
//  |                 |                         |  |  |  |  |             +----------------+
//  |                 |<----Vote()--------------+  |  |  |  |
//  | Config/Replica  |                            |  |  |  |             +----------------+
//  |                 |<----NewView()--------------+  |  |  +-Store()---> |                |
//  |                 |                               |  |                |   BlockChain   |
//  |                 |<----Fetch()-------------------+  +----Get()-----> |                |
//  +-----------------+                                                   +----------------+
//
// The `Consensus` interface is the "core" of the system, and it is the part that implements the consensus algorithm.
// The `OnDeliver()`, `OnPropose()`, `OnVote()`, and `OnNewView()` methods should be called by some backend service to
// deliver messages to the Consensus algorithm. The `Server` struct in the `backend/gorums` package is an example of
// such a service.
package hotstuff

import (
	"context"
	"encoding/base64"
)

// ID uniquely identifies a replica
type ID uint32

// View uniquely identifies a view
type View uint64

// Hash is a SHA256 hash
type Hash [32]byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h[:])
}

// Command is a client request to be executed by the consensus protocol
type Command string

//go:generate mockgen -destination=internal/mocks/cmdqueue_mock.go -package=mocks . CommandQueue

// CommandQueue is a queue of commands to be proposed
type CommandQueue interface {
	// GetCommand returns the next command to be proposed.
	GetCommand() *Command
}

// ToBytes is an object that can be converted into bytes for the purposes of hashing, etc.
type ToBytes interface {
	// ToBytes returns the object as bytes
	ToBytes() []byte
}

// PublicKey is the public part of a replica's key pair
type PublicKey interface{}

// Credentials are used to authenticate communication between replicas
type Credentials interface{}

// PrivateKey is the private part of a replica's key pair
type PrivateKey interface {
	// PublicKey returns the public key associated with this private key
	PublicKey() PublicKey
}

// Signature is a signature of a block
type Signature interface {
	ToBytes
	// Signer returns the ID of the replica that generated the signature.
	Signer() ID
}

// PartialCert is a certificate for a block created by a single replica
type PartialCert interface {
	ToBytes
	// Signature returns the signature
	Signature() Signature
	// BlockHash returns the hash of the block that was signed
	BlockHash() Hash
}

// QuorumCert is a certificate for a Block created by a quorum of replicas
type QuorumCert interface {
	ToBytes
	// BlockHash returns the hash of the block for which the certificate was created
	BlockHash() Hash
}

// Signer implements the methods requried to create signatures and certificates
type Signer interface {
	// Sign signs a single block and returns the signature
	Sign(block *Block) (cert PartialCert, err error)
	// CreateQuourmCert creates a from a list of partial certificates
	CreateQuorumCert(block *Block, signatures []PartialCert) (cert QuorumCert, err error)
}

// Verifier implements the methods required to verify partial and quorum certificates
type Verifier interface {
	// VerifyPartialCert verifies a single partial certificate
	VerifyPartialCert(cert PartialCert) bool
	// VerifyQuorumCert verifies a quorum certificate
	VerifyQuorumCert(qc QuorumCert) bool
}

//go:generate mockgen -destination=internal/mocks/replica_mock.go -package=mocks . Replica

// Replica implements the methods that communicate with another replica
type Replica interface {
	// ID returns the replica's id
	ID() ID
	// PublicKey returns the replica's public key
	PublicKey() PublicKey
	// Vote sends the partial certificate to the other replica
	Vote(cert PartialCert)
	// NewView sends the quorum certificate to the other replica
	NewView(qc QuorumCert)
	// Deliver sends the block to the other replica
	Deliver(block *Block)
}

// BlockChain is a datastructure that stores a chain of blocks
type BlockChain interface {
	// Store stores a block in the blockchain
	Store(*Block)
	// Get retrieves a block given its hash
	Get(Hash) (*Block, bool)
}

//go:generate mockgen -destination=internal/mocks/consensus_mock.go -package=mocks . Consensus

// Consensus implements a consensus protocol
type Consensus interface {
	// Config returns the configuration of this replica
	Config() Config
	// LastVote returns the view in which the replica last voted
	LastVote() View
	// HighQC returns the highest QC known to the replica
	HighQC() QuorumCert
	// Leaf returns the last proposed block
	Leaf() *Block
	// BlockChain returns the datastructure containing the blocks known to the replica
	BlockChain() BlockChain
	// CreateDummy inserts a dummy block at View+1.
	// This is useful when a view must be skipped.
	CreateDummy()
	// Propose starts a new proposal
	Propose()
	// NewView sends a NewView message to the next leader
	NewView()
	// OnPropose handles an incoming proposal
	OnPropose(block *Block)
	// OnVote handles an incoming vote
	OnVote(cert PartialCert)
	// OnNewView handles an incoming NewView
	OnNewView(qc QuorumCert)
	// OnDeliver handles an incoming block
	OnDeliver(block *Block)
}

//go:generate mockgen -destination=internal/mocks/config_mock.go -package=mocks . Config

// Config holds information about Replicas and provides methods to send messages to the replicas
type Config interface {
	// ID returns the id of this replica
	ID() ID
	// PrivateKey returns the id of this replica
	PrivateKey() PrivateKey
	// Replicas returns all of the replicas in the configuration
	Replicas() map[ID]Replica
	// Replica returns a replica if present in the configuration
	Replica(ID) (replica Replica, ok bool)
	// Len returns the number of replicas in the configuration
	Len() int
	// QuorumSize returns the size of a quorum
	QuorumSize() int
	// Propose sends the block to all replicas in the configuration
	Propose(block *Block)
	// Fetch requests a block from all the replicas in the configuration
	Fetch(ctx context.Context, hash Hash)
}

// LeaderRotation implements a leader rotation scheme
type LeaderRotation interface {
	// GetLeader returns the id of the leader in the given view
	GetLeader(View) ID
}

//go:generate mockgen -destination=internal/mocks/acceptor_mock.go -package=mocks . Acceptor

// Acceptor is the mechanism that decides wether a command should be accepted by the replica
type Acceptor interface {
	// Accept returns true if the replica should accept the command, false otherwise
	Accept(Command) bool
}

//go:generate mockgen -destination=internal/mocks/executor_mock.go -package=mocks . Executor

// Executor executes a command
type Executor interface {
	// Exec executes the given command
	Exec(Command)
}

//go:generate mockgen -destination=internal/mocks/synchronizer_mock.go -package=mocks . ViewSynchronizer

// ViewSynchronizer synchronizes replicas to the same view
type ViewSynchronizer interface {
	LeaderRotation
	// OnPropose should be called when a replica has received a new valid proposal.
	OnPropose()
	// OnFinishQC should be called when a replica has created a new qc
	OnFinishQC()
	// OnNewView should be called when a replica receives a valid NewView message
	OnNewView()
	// Init gives the synchronizer a consensus instance to synchronize
	Init(Consensus)
	// Start starts the synchronizer
	Start()
	// Stop stops the synchronizer
	Stop()
}
