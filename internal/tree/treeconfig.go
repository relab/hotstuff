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

// CreateTree creates the tree configuration.
// Currently only fault free tree configuration is supported.
func CreateTree(myID hotstuff.ID, bf int, ids []hotstuff.ID) *Tree {
	if bf < 2 {
		panic("Branch factor must be greater than 1")
	}
	if slices.Index(ids, myID) == -1 {
		panic("Replica ID not found in tree configuration")
	}

	// compute height of the tree
	temp := len(ids) - 1 // root
	height := 1
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

// TreeHeight returns the height of the full tree.
func (t *Tree) TreeHeight() int {
	return t.height
}

// Parent returns the ID of the parent of this tree's replica and true.
// If this tree's replica is the root, the root's ID is returned
// and false to indicate it does not have a parent.
func (t *Tree) Parent() (hotstuff.ID, bool) {
	myPos := t.replicaPosition(t.id)
	if myPos == 0 {
		return t.id, false
	}
	parentPos := (myPos - 1) / t.branchFactor
	return t.posToIDMapping[parentPos], true
}

// IsRoot return true if the replica is at root of the tree.
func (t *Tree) IsRoot(replicaID hotstuff.ID) bool {
	return t.replicaPosition(replicaID) == 0
}

// ReplicaChildren returns the children of this tree's replica, if any.
func (t *Tree) ReplicaChildren() []hotstuff.ID {
	return t.ChildrenOf(t.id)
}

// ChildrenOf returns the children of a specific replica.
func (t *Tree) ChildrenOf(replicaID hotstuff.ID) []hotstuff.ID {
	replicaPos := t.replicaPosition(replicaID)
	if replicaPos == -1 {
		return nil
	}
	children := make([]hotstuff.ID, 0)
	for i := 1; i <= t.branchFactor; i++ {
		childPos := (t.branchFactor * replicaPos) + i
		if t.isWithInIndex(childPos) {
			children = append(children, t.posToIDMapping[childPos])
		} else {
			break
		}
	}
	return children
}

// ReplicaHeight returns the height of this tree's replica.
func (t *Tree) ReplicaHeight() int {
	return t.heightOf(t.id)
}

// PeersOf returns the sibling peers of given ID, if any.
func (t *Tree) PeersOf(replicaID hotstuff.ID) []hotstuff.ID {
	if t.IsRoot(replicaID) {
		return nil
	}
	parent, ok := t.Parent()
	if !ok {
		return nil
	}
	return t.ChildrenOf(parent)
}

// SubTree returns all subtree replicas of this tree's replica.
func (t *Tree) SubTree() []hotstuff.ID {
	children := t.ChildrenOf(t.id)
	if len(children) == 0 {
		return nil
	}
	subTreeReplicas := make([]hotstuff.ID, len(children))
	copy(subTreeReplicas, children)
	for i := 0; i < len(subTreeReplicas); i++ {
		node := subTreeReplicas[i]
		newChildren := t.ChildrenOf(node)
		subTreeReplicas = append(subTreeReplicas, newChildren...)
	}
	return subTreeReplicas
}

// heightOf returns the height from the given replica's vantage point.
func (t *Tree) heightOf(replicaID hotstuff.ID) int {
	if t.IsRoot(replicaID) {
		return t.height
	}
	replicaPos := t.replicaPosition(replicaID)
	if replicaPos == -1 {
		return 0
	}
	startLimit := 0
	endLimit := 0
	for i := 1; i < t.height; i++ {
		startLimit = startLimit + int(math.Pow(float64(t.branchFactor), float64(i-1)))
		endLimit = endLimit + int(math.Pow(float64(t.branchFactor), float64(i)))
		if replicaPos >= startLimit && replicaPos <= endLimit {
			return t.height - i
		}
	}
	return 0
}

func (t *Tree) replicaPosition(id hotstuff.ID) int {
	return slices.Index(t.posToIDMapping, id)
}

func (t *Tree) isWithInIndex(position int) bool {
	return position < len(t.posToIDMapping)
}
