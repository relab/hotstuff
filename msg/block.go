package msg

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/relab/hotstuff"
)

// Block contains a proposed "command", metadata for the protocol, and a link to the "parent" block.
// type BlockOld struct {
// 	// keep a copy of the hash to avoid hashing multiple times
// 	hash     Hash
// 	parent   Hash
// 	proposer hotstuff.ID
// 	cmd      Command
// 	cert     QuorumCert
// 	view     View
// }

// type Block struct {
// 	Parent   []byte      `protobuf:"bytes,1,opt,name=Parent,proto3" json:"Parent,omitempty"`
// 	QC       *QuorumCert `protobuf:"bytes,2,opt,name=QC,proto3" json:"QC,omitempty"`
// 	View     uint64      `protobuf:"varint,3,opt,name=View,proto3" json:"View,omitempty"`
// 	Command  []byte      `protobuf:"bytes,4,opt,name=Command,proto3" json:"Command,omitempty"`
// 	Proposer uint32      `protobuf:"varint,5,opt,name=Proposer,proto3" json:"Proposer,omitempty"`
// }

// NewBlock creates a new Block
func NewBlock(parent Hash, cert *QuorumCert, cmd Command, view View, proposer hotstuff.ID) *Block {
	b := &Block{
		Parent:   parent[:],
		QC:       cert,
		Command:  []byte(cmd),
		View:     uint64(view),
		Proposer: uint32(proposer),
	}
	b.Hash = b.computeHash()
	return b
}

func (b *Block) BString() string {
	return fmt.Sprintf(
		"Block{ hash: %.6s parent: %.6s, proposer: %d, view: %d , cert: %v }",
		string(b.GetHashBytes()),
		b.ParentHash().String(),
		b.Proposer,
		b.View,
		b.QC,
	)
}

// // Hash returns the hash of the Block
// func (b *Block) Hash() Hash {
// 	// TODO Should ideally cache the hash, rather than computing it every time
// 	// y, _ := proto.Marshal(b)
// 	// return Hash(sha256.Sum256(y))
// 	return Hash{}
// }

// ProposerID returns the id of the replica who proposed the block.
func (b *Block) ProposerID() hotstuff.ID {
	return hotstuff.ID(b.Proposer)
}

// ParentHash returns the hash of the parent Block
func (b *Block) ParentHash() Hash {
	return *(*Hash)(b.Parent)
}

// Cmd returns the command
func (b *Block) Cmd() Command {
	return Command(b.Command)
}

// QuorumCert returns the quorum certificate in the block
func (b *Block) QuorumCert() *QuorumCert {
	return (*b).QC
}

// BView returns the view in which the Block was proposed
func (b *Block) BView() View {
	return View(b.View)
}

// // ToBytes returns the raw byte form of the Block, to be used for hashing, etc.
// func (b *Block) ToBytes() []byte {
// 	buf, _ := proto.Marshal(b)
// 	return buf
// }

func (b *Block) GetHashBytes() []byte {
	return b.Hash
}

func (b *Block) GetBlockHash() Hash {
	var hash Hash
	copy(hash[:], b.Hash)
	for i, b := range b.Hash {
		hash[i] = b
	}
	return hash
}

func (b *Block) computeHash() []byte {
	hash := make([]byte, 0)
	for _, b := range sha256.Sum256(b.ToBytes()) {
		hash = append(hash, b)
	}
	return hash
}
func (b *Block) ToBytes() []byte {
	buf := b.Parent[:]
	var proposerBuf [4]byte
	binary.LittleEndian.PutUint32(proposerBuf[:], uint32(b.Proposer))
	buf = append(buf, proposerBuf[:]...)
	var viewBuf [8]byte
	binary.LittleEndian.PutUint64(viewBuf[:], uint64(b.View))
	buf = append(buf, viewBuf[:]...)
	buf = append(buf, []byte(b.Command)...)
	buf = append(buf, b.QC.ToBytes()...)
	return buf
}
