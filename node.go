package hotstuff

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

// NodeStorage provides a means to store a Node based on its hash
type NodeStorage interface {
	Put(*Node)
	Get(NodeHash) (*Node, bool)
	NodeOf(*QuorumCert) (*Node, bool)
	ParentOf(*Node) (*Node, bool)
	GarbageCollectNodes(int)
}

// NodeHash represents a SHA256 hashsum of a Node
type NodeHash [32]byte

func (h NodeHash) String() string {
	return hex.EncodeToString(h[:])
}

// Node represents a node in the tree of commands
type Node struct {
	hash       *NodeHash
	ParentHash NodeHash
	Commands   []Command
	Justify    *QuorumCert
	Height     int
	Committed  bool
}

func (n Node) String() string {
	return fmt.Sprintf("Node{Parent: %.8s, Justify: %s, Height: %d, Committed: %v}",
		n.ParentHash, n.Justify, n.Height, n.Committed)
}

// Hash returns a hash digest of the node.
func (n Node) Hash() NodeHash {
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
		s256.Write(n.Justify.toBytes())
	}

	for _, cmd := range n.Commands {
		s256.Write([]byte(cmd))
	}

	n.hash = new(NodeHash)
	sum := s256.Sum(nil)
	copy(n.hash[:], sum)

	return *n.hash
}

// MapStorage is a simple implementation of NodeStorage that uses a concurrent map.
type MapStorage struct {
	// TODO: Experiment with RWMutex
	mut   sync.Mutex
	nodes map[NodeHash]*Node
}

// NewMapStorage returns a new instance of MapStorage
func NewMapStorage() *MapStorage {
	return &MapStorage{
		nodes: make(map[NodeHash]*Node),
	}
}

// Put inserts a node into the map
func (s *MapStorage) Put(node *Node) {
	s.mut.Lock()
	defer s.mut.Unlock()

	hash := node.Hash()
	if _, ok := s.nodes[hash]; !ok {
		s.nodes[hash] = node
	}
}

// Get gets a node from the map based on its hash.
func (s *MapStorage) Get(hash NodeHash) (node *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	node, ok = s.nodes[hash]
	return
}

// NodeOf returns the node associated with the quorum cert
func (s *MapStorage) NodeOf(qc *QuorumCert) (node *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	node, ok = s.nodes[qc.NodeHash]
	return
}

// ParentOf returns the parent of the given node
func (s *MapStorage) ParentOf(child *Node) (parent *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	parent, ok = s.nodes[child.ParentHash]
	return
}

// GarbageCollectNodes dereferences old nodes that are no longer needed
func (s *MapStorage) GarbageCollectNodes(currentVeiwHeigth int) {
	s.mut.Lock()
	defer s.mut.Unlock()

	var deleteAncestors func(node *Node)

	deleteAncestors = func(node *Node) {
		parent, ok := s.nodes[node.ParentHash]
		if ok {
			deleteAncestors(parent)
		}
		delete(s.nodes, node.Hash())
	}

	for _, n := range s.nodes {
		if n.Height+50 < currentVeiwHeigth {
			deleteAncestors(n)
		}
	}
}
