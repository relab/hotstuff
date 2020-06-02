package data

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

// Command is the client data that is processed by HotStuff
type Command string

// BlockStorage provides a means to store a block based on its hash
type BlockStorage interface {
	Put(*Block)
	Get(BlockHash) (*Block, bool)
	BlockOf(*QuorumCert) (*Block, bool)
	ParentOf(*Block) (*Block, bool)
	GarbageCollectBlocks(int)
}

// BlockHash represents a SHA256 hashsum of a Block
type BlockHash [32]byte

func (h BlockHash) String() string {
	return hex.EncodeToString(h[:])
}

// Block represents a block in the tree of commands
type Block struct {
	hash       *BlockHash
	ParentHash BlockHash
	Commands   []Command
	Justify    *QuorumCert
	Height     int
	Committed  bool
}

func (n Block) String() string {
	return fmt.Sprintf("Block{Parent: %.8s, Justify: %s, Height: %d, Committed: %v}",
		n.ParentHash, n.Justify, n.Height, n.Committed)
}

// Hash returns a hash digest of the block.
func (n Block) Hash() BlockHash {
	// return cached hash if available
	if n.hash != nil {
		return *n.hash
	}

	s256 := sha256.New()

	s256.Write(n.ParentHash[:])

	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, uint64(n.Height))
	s256.Write(height[:])

	if n.Justify != nil {
		s256.Write(n.Justify.ToBytes())
	}

	for _, cmd := range n.Commands {
		s256.Write([]byte(cmd))
	}

	n.hash = new(BlockHash)
	sum := s256.Sum(nil)
	copy(n.hash[:], sum)

	return *n.hash
}

// MapStorage is a simple implementation of BlockStorage that uses a concurrent map.
type MapStorage struct {
	// TODO: Experiment with RWMutex
	mut    sync.Mutex
	blocks map[BlockHash]*Block
}

// NewMapStorage returns a new instance of MapStorage
func NewMapStorage() *MapStorage {
	return &MapStorage{
		blocks: make(map[BlockHash]*Block),
	}
}

// Put inserts a block into the map
func (s *MapStorage) Put(block *Block) {
	s.mut.Lock()
	defer s.mut.Unlock()

	hash := block.Hash()
	if _, ok := s.blocks[hash]; !ok {
		s.blocks[hash] = block
	}
}

// Get gets a block from the map based on its hash.
func (s *MapStorage) Get(hash BlockHash) (block *Block, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	block, ok = s.blocks[hash]
	return
}

// BlockOf returns the block associated with the quorum cert
func (s *MapStorage) BlockOf(qc *QuorumCert) (block *Block, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	block, ok = s.blocks[qc.BlockHash]
	return
}

// ParentOf returns the parent of the given Block
func (s *MapStorage) ParentOf(child *Block) (parent *Block, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	parent, ok = s.blocks[child.ParentHash]
	return
}

// GarbageCollectBlocks dereferences old Blocks that are no longer needed
func (s *MapStorage) GarbageCollectBlocks(currentVeiwHeigth int) {
	s.mut.Lock()
	defer s.mut.Unlock()

	var deleteAncestors func(block *Block)

	deleteAncestors = func(block *Block) {
		parent, ok := s.blocks[block.ParentHash]
		if ok {
			deleteAncestors(parent)
		}
		delete(s.blocks, block.Hash())
	}

	for _, n := range s.blocks {
		if n.Height+50 < currentVeiwHeigth {
			deleteAncestors(n)
		}
	}
}
