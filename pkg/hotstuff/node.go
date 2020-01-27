package hotstuff

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/relab/hotstuff/pkg/proto"
	"sync"
)

// NodeStorage provides a means to store a Node based on its hash
type NodeStorage interface {
	Put(*Node)
	Get(NodeHash) (*Node, bool)
	Node(*QuorumCert) (*Node, bool)
	Parent(*Node) (*Node, bool)
}

// NodeHash represents a SHA256 hashsum of a Node
type NodeHash [32]byte

// Node represents a node in the tree of commands
type Node struct {
	ParentHash NodeHash
	Command    []byte
	Justify    *QuorumCert
	Height     int
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
	toHash = append(toHash, n.Justify.toBytes()...)
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

// Put adds a node to the map.
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

// Node returns the node associated with the quorum cert
func (s *MapStorage) Node(qc *QuorumCert) (node *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	node, ok = s.nodes[qc.hash]
	return
}

// Parent returns the parent of the given node
func (s *MapStorage) Parent(child *Node) (parent *Node, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	parent, ok = s.nodes[child.ParentHash]
	return
}
