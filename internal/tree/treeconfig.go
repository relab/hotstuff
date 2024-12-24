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
	childStart := replicaPos*t.branchFactor + 1
	if childStart >= len(t.posToIDMapping) {
		// no children since start is beyond slice length
		return nil
	}
	childEnd := childStart + t.branchFactor
	if childEnd > len(t.posToIDMapping) {
		// clamp to the slice length
		childEnd = len(t.posToIDMapping)
	}
	return t.posToIDMapping[childStart:childEnd]
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

	// startLvl is the "first index" in the current level,
	// levelCount is how many nodes are in this level.
	//
	// With branchFactor = n, the level sizes grow as:
	// Level 0: 1 node (the root)
	// Level 1: n nodes
	// Level 2: n^2 nodes
	// ...
	// We start at level 1, since the root returns early.
	startLvl := 1              // index of the first node at level 1
	lvlCount := t.branchFactor // number of nodes at level 1

	for lvl := 1; lvl < t.height; lvl++ {
		endLvl := startLvl + lvlCount // one-past the last node at this level
		if replicaPos >= startLvl && replicaPos < endLvl {
			// replicaPos is in [startLvl, endLvl): t.height-lvl is the height.
			return t.height - lvl
		}
		// Move to the next level:
		startLvl = endLvl
		lvlCount *= t.branchFactor
	}
	return 0
}

func (t *Tree) replicaPosition(id hotstuff.ID) int {
	return slices.Index(t.posToIDMapping, id)
}
