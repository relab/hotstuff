package blockchain

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/relab/hotstuff"
)

type block struct {
	// keep a copy of the hash to avoid hashing multiple times
	hash     *hotstuff.Hash
	parent   hotstuff.Hash
	proposer hotstuff.ID
	cmd      hotstuff.Command
	cert     hotstuff.QuorumCert
	view     hotstuff.View
}

func NewBlock(parent hotstuff.Hash, cert hotstuff.QuorumCert, cmd hotstuff.Command, view hotstuff.View, proposer hotstuff.ID) hotstuff.Block {
	return &block{
		parent:   parent,
		cert:     cert,
		cmd:      cmd,
		view:     view,
		proposer: proposer,
	}
}

func (b *block) hashSlow() hotstuff.Hash {
	return sha256.Sum256(b.ToBytes())
}

// Hash returns the hash of the block
func (b *block) Hash() *hotstuff.Hash {
	if b.hash == nil {
		b.hash = new(hotstuff.Hash)
		*b.hash = b.hashSlow()
	}
	return b.hash
}

// Proposer returns the id of the proposer
func (b *block) Proposer() hotstuff.ID {
	return b.proposer
}

// Parent returns the hash of the parent block
func (b *block) Parent() *hotstuff.Hash {
	return &b.parent
}

// Command returns the command
func (b *block) Command() hotstuff.Command {
	return b.cmd
}

// Certificate returns the certificate that this block references
func (b *block) Certificate() hotstuff.QuorumCert {
	return b.cert
}

// View returns the view in which the block was proposed
func (b *block) View() hotstuff.View {
	return b.view
}

func (b *block) ToBytes() []byte {
	buf := b.parent[:]
	var proposerBuf [4]byte
	binary.LittleEndian.PutUint32(proposerBuf[:], uint32(b.proposer))
	buf = append(buf, proposerBuf[:]...)
	var viewBuf [8]byte
	binary.LittleEndian.PutUint64(viewBuf[:], uint64(b.view))
	buf = append(buf, viewBuf[:]...)
	buf = append(buf, []byte(b.cmd)...)
	buf = append(buf, b.cert.ToBytes()...)
	return buf
}

var _ hotstuff.Block = (*block)(nil)
