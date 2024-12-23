package tree

import (
	"math"
	"slices"

	"github.com/relab/hotstuff"
)

// Tree implements a fault free tree configuration.
type Tree struct {
	id             hotstuff.ID
	height         int
	posToIDMapping []hotstuff.ID
	branchFactor   int
}

// CreateTree creates the tree configuration, currently only fault free tree configuration is supported.
func CreateTree(myID hotstuff.ID, bf int, ids []hotstuff.ID) *Tree {

	temp := len(ids)
	temp = temp - 1 // root
	height := 1
	if bf < 2 {
		panic("branch factor should be greater than 1")
	}
	index := slices.Index(ids, myID)
	if index == -1 {
		panic("replica ID not found in tree configuration")
	}
	for i := 1; temp > 0; i++ {
		temp = temp - int(math.Pow(float64(bf), float64(i)))
		height++
	}
	return &Tree{
		id:             myID,
		height:         height,
		branchFactor:   bf,
		posToIDMapping: ids,
	}
}

func (t *Tree) GetTreeHeight() int {
	return t.height
}

func (t *Tree) replicaPosition(replicaId hotstuff.ID) int {
	return slices.Index(t.posToIDMapping, replicaId)
}

// Parent fetches the ID of the parent, if root, returns itself
// bool value indicates there is no parent for this id
func (t *Tree) Parent() (hotstuff.ID, bool) {
	myPos := t.replicaPosition(t.id)
	if myPos == -1 {
		return hotstuff.ID(0), false
	}
	if myPos == 0 {
		return t.id, false
	}
	parentPos := (myPos - 1) / t.branchFactor
	return t.posToIDMapping[parentPos], true
}

// ChildrenOf returns the children of the replica, if any.
func (t *Tree) ChildrenOf() []hotstuff.ID {
	return t.ChildrenOfNode(t.id)
}

func (t *Tree) isWithInIndex(position int) bool {
	return position < len(t.posToIDMapping)
}

// IsRoot return true if the replica is at root of the tree.
func (t *Tree) IsRoot(nodeID hotstuff.ID) bool {
	return t.replicaPosition(nodeID) == 0
}

// ChildrenOfNode returns the children of a specific replica.
func (t *Tree) ChildrenOfNode(nodeID hotstuff.ID) []hotstuff.ID {
	children := make([]hotstuff.ID, 0)
	nodePos := t.replicaPosition(nodeID)
	if nodePos == -1 {
		return children
	}
	for i := 1; i <= t.branchFactor; i++ {
		childPos := (t.branchFactor * nodePos) + i
		if t.isWithInIndex(childPos) {
			children = append(children, t.posToIDMapping[childPos])
		} else {
			break
		}
	}
	return children
}

// getHeight returns the height from the given replica's vantage point.
func (t *Tree) getHeight(nodeID hotstuff.ID) int {
	if t.IsRoot(nodeID) {
		return t.height
	}
	nodePos := t.replicaPosition(nodeID)
	if nodePos == -1 {
		return 0
	}
	startLimit := 0
	endLimit := 0
	for i := 1; i < t.height; i++ {
		startLimit = startLimit + int(math.Pow(float64(t.branchFactor), float64(i-1)))
		endLimit = endLimit + int(math.Pow(float64(t.branchFactor), float64(i)))
		if nodePos >= startLimit && nodePos <= endLimit {
			return t.height - i
		}
	}
	return 0
}

// GetHeight returns the height of the replica
func (t *Tree) GetHeight() int {
	return t.getHeight(t.id)
}

// PeersOf returns the peers of given ID, if any.
func (t *Tree) PeersOf(nodeID hotstuff.ID) []hotstuff.ID {
	peers := make([]hotstuff.ID, 0)
	if t.IsRoot(nodeID) {
		return peers
	}
	parent, ok := t.Parent()
	if !ok {
		return peers
	}
	return t.ChildrenOfNode(parent)
}

// SubTree returns all the nodes of its subtree.
func (t *Tree) SubTree() []hotstuff.ID {
	nodeID := t.id
	subTreeNodes := make([]hotstuff.ID, 0)
	children := t.ChildrenOfNode(nodeID)
	queue := make([]hotstuff.ID, 0)
	queue = append(queue, children...)
	subTreeNodes = append(subTreeNodes, children...)
	if len(children) == 0 {
		return subTreeNodes
	}
	for len(queue) > 0 {
		child := queue[0]
		queue = queue[1:]
		children := t.ChildrenOfNode(child)
		subTreeNodes = append(subTreeNodes, children...)
		queue = append(queue, children...)
	}
	return subTreeNodes
}
