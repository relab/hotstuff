package tree

import (
	"slices"

	"github.com/relab/hotstuff"
)

// Tree contains the local replica's ID which must be part of the tree.
// Once created, the tree is immutable.
type Tree struct {
	id           hotstuff.ID
	height       int
	branchFactor int
	treePosToID  []hotstuff.ID
}

// CreateTree creates the tree configuration.
func CreateTree(myID hotstuff.ID, bf int, ids []hotstuff.ID) Tree {
	if bf < 2 {
		panic("Branch factor must be greater than 1")
	}
	if slices.Index(ids, myID) == -1 {
		panic("Replica ID not found in tree configuration")
	}

	return Tree{
		id:           myID,
		height:       treeHeight(len(ids), bf),
		branchFactor: bf,
		treePosToID:  ids,
	}
}

// treeHeight returns the height of a tree with numNodes and branch factor bf.
func treeHeight(numNodes, bf int) (height int) {
	// number of nodes at the current level (root = 1)
	levelSize := 1

	// subtract the number of nodes at each level until we run out of nodes.
	for numNodes > 0 {
		numNodes -= levelSize
		levelSize *= bf // next level has bf times more nodes
		height++
	}
	return height
}

// TreeHeight returns the height of the full tree.
func (t Tree) TreeHeight() int {
	return t.height
}

// Parent returns the ID of the parent of this tree's replica and true.
// If this tree's replica is the root, the root's ID is returned
// and false to indicate it does not have a parent.
func (t Tree) Parent() (hotstuff.ID, bool) {
	myPos := t.replicaPosition(t.id)
	if myPos == 0 {
		return t.id, false
	}
	parentPos := (myPos - 1) / t.branchFactor
	return t.treePosToID[parentPos], true
}

// Root returns the ID of the root of the tree.
func (t Tree) Root() hotstuff.ID {
	return t.treePosToID[0]
}

// IsRoot return true if the replica is at root of the tree.
func (t Tree) IsRoot(replicaID hotstuff.ID) bool {
	return t.replicaPosition(replicaID) == 0
}

// ReplicaChildren returns the children of this tree's replica, if any.
func (t Tree) ReplicaChildren() []hotstuff.ID {
	return t.ChildrenOf(t.id)
}

// ChildrenOf returns the children of a specific replica.
func (t Tree) ChildrenOf(replicaID hotstuff.ID) []hotstuff.ID {
	replicaPos := t.replicaPosition(replicaID)
	if replicaPos == -1 {
		// safe since nil slices are equal to empty slices when iterating.
		return nil
	}
	childStart := replicaPos*t.branchFactor + 1
	if childStart >= len(t.treePosToID) {
		// no children since start is beyond slice length
		return nil
	}
	childEnd := childStart + t.branchFactor
	if childEnd > len(t.treePosToID) {
		// clamp to the slice length
		childEnd = len(t.treePosToID)
	}
	return t.treePosToID[childStart:childEnd]
}

// ReplicaHeight returns the height of this tree's replica.
func (t Tree) ReplicaHeight() int {
	return t.heightOf(t.id)
}

// PeersOf returns the sibling peers of the tree's replica,
// unless the replica is the root, in which case there are no siblings.
func (t Tree) PeersOf() []hotstuff.ID {
	parent, ok := t.Parent()
	if !ok {
		// the tree's replica is the root, hence it has no siblings.
		return nil
	}
	return t.ChildrenOf(parent)
}

// SubTree returns all subtree replicas of this tree's replica.
func (t Tree) SubTree() []hotstuff.ID {
	children := t.ChildrenOf(t.id)
	if len(children) == 0 {
		// safe since nil slices are equal to empty slices when iterating.
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
func (t Tree) heightOf(replicaID hotstuff.ID) int {
	if t.IsRoot(replicaID) {
		return t.height
	}
	replicaPos := t.replicaPosition(replicaID)
	// this -1 check is only necessary if we export this method.
	// it is not necessary for internal use since then we know the replica is in the tree.
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

func (t Tree) replicaPosition(id hotstuff.ID) int {
	return slices.Index(t.treePosToID, id)
}

// DefaultTreePos returns the tree positions of the replicas.
func DefaultTreePos(size int) []hotstuff.ID {
	ids := make([]hotstuff.ID, size)
	for i := range size {
		ids[i] = hotstuff.ID(i + 1)
	}
	return ids
}

// DefaultTreePosUint32 returns the tree positions of the replicas.
func DefaultTreePosUint32(size int) []uint32 {
	ids := make([]uint32, size)
	for i := range size {
		ids[i] = uint32(i + 1)
	}
	return ids
}
