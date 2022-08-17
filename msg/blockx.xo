package msg

import (
	"crypto/sha256"
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/msg/hotstuffpb"
	"google.golang.org/protobuf/proto"
)

// TO READ AGAIN: https://talks.golang.org/2016/refactor.article
// https://github.com/golang/proposal/blob/master/design/18130-type-alias.md

type BlockX hotstuffpb.Block

type BlockY struct {
	*hotstuffpb.Block
	hash Hash
}

// NewBlockX creates a new Block
func NewBlockX(parent Hash, cert *hotstuffpb.QuorumCert, cmd Command, view View, proposer hotstuff.ID) *BlockX {
	x := &hotstuffpb.Block{
		Parent:   parent[:],
		QC:       cert,
		Command:  []byte(cmd),
		View:     uint64(view),
		Proposer: uint32(proposer),
	}
	return (*BlockX)(x)
}

func NewBlockY(parent Hash, cert *hotstuffpb.QuorumCert, cmd Command, view View, proposer hotstuff.ID) *BlockY {
	x := &BlockY{
		Block: &hotstuffpb.Block{
			Parent:   parent[:],
			QC:       cert,
			Command:  []byte(cmd),
			View:     uint64(view),
			Proposer: uint32(proposer),
		},
	}
	// cache the hash immediately because it is too racy to do it in Hash()
	y, _ := proto.Marshal(x)
	x.hash = sha256.Sum256(y)
	return (*BlockY)(x)
}

func (b *BlockX) String() string {
	return fmt.Sprintf(
		"Block{ hash: %.6s parent: %.6s, proposer: %d, view: %d , cert: %v }",
		b.Hash().String(),
		b.ParentX().String(),
		b.Proposer,
		b.View,
		b.QC,
	)
}

// Hash returns the hash of the Block
func (b *BlockX) Hash() Hash {
	x := (*hotstuffpb.Block)(b)
	y, _ := proto.Marshal(x)
	return Hash(sha256.Sum256(y))
}

// Proposer returns the id of the replica who proposed the block.
func (b *BlockX) ProposerX() hotstuff.ID {
	return hotstuff.ID(b.Proposer)
}

func (b *BlockX) ParentX() Hash {
	// converts []byte to Hash ([32]byte array)
	return *(*Hash)(b.Parent)
}

// Command returns the command
func (b *BlockX) CommandX() Command {
	return Command(b.Command)
}

// QuorumCert returns the quorum certificate in the block
func (b *BlockX) QuorumCert() *hotstuffpb.QuorumCert {
	return b.QC
}

// View returns the view in which the Block was proposed
func (b *BlockX) ViewX() View {
	return View(b.View)
}

// ToBytes returns the raw byte form of the Block, to be used for hashing, etc.
func (b *BlockX) ToBytesX() []byte {
	x := (*hotstuffpb.Block)(b)
	buf, _ := proto.Marshal(x)
	return buf
}

// ToBytes returns the raw byte form of the Block, to be used for hashing, etc.
func (b *BlockY) ToBytesY() []byte {
	x := (*hotstuffpb.Block)(b.Block)
	buf, _ := proto.Marshal(x)
	return buf
}
