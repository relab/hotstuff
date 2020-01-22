package hotstuff

import (
	"crypto/sha256"
	"encoding/binary"
)

// NodeStorage provides a means to store a Node based on its hash
type NodeStorage interface {
	Put(Node)
	Get(string)
}

// Node represents a node in the tree of commands
type Node struct {
	ParentHash []byte
	Command    []byte
	Justify    QuorumCert
	Height     int
}

// Hash returns a hash digest of the node.
func (n Node) Hash() []byte {
	// add the fields that should be hashed to a single slice
	var toHash []byte
	toHash = append(toHash, n.ParentHash...)
	toHash = append(toHash, n.Command...)
	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, uint64(n.Height))
	toHash = append(toHash, height...)

	hash := sha256.New()

	return hash.Sum(toHash)
}
