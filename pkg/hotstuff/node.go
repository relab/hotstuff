package hotstuff

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/relab/hotstuff/pkg/proto"
)

// NodeStorage provides a means to store a Node based on its hash
type NodeStorage interface {
	Put(*Node)
	Get(NodeHash) (*Node, bool)
	NodeOf(*QuorumCert) (*Node, bool)
	ParentOf(*Node) (*Node, bool)
}

// NodeHash represents a SHA256 hashsum of a Node
type NodeHash [32]byte

func (h NodeHash) String() string {
	return hex.EncodeToString(h[:])
}

// Node represents a node in the tree of commands
type Node struct {
	ParentHash NodeHash
	Command    []byte
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
	// add the fields that should be hashed to a single slice
	var toHash []byte
	toHash = append(toHash, n.ParentHash[:]...)
	toHash = append(toHash, n.Command...)
	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, uint64(n.Height))
	toHash = append(toHash, height...)
	// TODO: Figure out if this ever occurs in practice (genesis node?)
	if n.Justify != nil {
		toHash = append(toHash, n.Justify.toBytes()...)
	}
	return sha256.Sum256(toHash)
}

func (n Node) toProto() *proto.HSNode {
	return &proto.HSNode{
		ParentHash: n.ParentHash[:],
		Command:    n.Command,
		QC:         n.Justify.toProto(),
		Height:     int64(n.Height),
	}
}

func nodeFromProto(pn *proto.HSNode) *Node {
	n := &Node{
		Command: pn.GetCommand(),
		Justify: quorumCertFromProto(pn.GetQC()),
		Height:  int(pn.Height),
	}
	copy(n.ParentHash[:], pn.GetParentHash())
	return n
}

// MapStorage is a simple implementation of NodeStorage that uses a concurrent map.
type MapStorage struct {
	mut   sync.Mutex
	nodes map[NodeHash]*Node
}

func NewMapStorage() *MapStorage {
	return &MapStorage{
		nodes: make(map[NodeHash]*Node),
	}
}

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

// NodeFor returns the node associated with the quorum cert
func (s *MapStorage) NodeOf(qc *QuorumCert) (node *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	node, ok = s.nodes[qc.hash]
	return
}

// Parent returns the parent of the given node
func (s *MapStorage) ParentOf(child *Node) (parent *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	parent, ok = s.nodes[child.ParentHash]
	return
}
