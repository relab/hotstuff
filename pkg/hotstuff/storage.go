package hotstuff

// NodeStorage provides a means to store a Node based on its hash
type NodeStorage interface {
	Put(Node)
	Get(string)
}

// Node represents a node in the tree of commands
type Node struct {
	parent  *Node
	command []byte
	qc      QuorumCert
}
