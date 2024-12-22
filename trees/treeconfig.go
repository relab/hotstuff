package trees

import (
	"fmt"
	"math"
	"slices"

	"github.com/relab/hotstuff"
)

// Tree implements a fault free tree configuration.
type Tree struct {
	id                hotstuff.ID
	configurationSize int
	height            int
	posToIDMapping    []hotstuff.ID
	branchFactor      int
}

// CreateTree creates the tree configuration, currently only fault free tree configuration is supported.
func CreateTree(configurationSize int, myID hotstuff.ID, bf int) *Tree {
	if configurationSize <= 0 {
		return nil
	}
	temp := configurationSize
	temp = temp - 1 // root
	height := 1
	for i := 1; temp > 0; i++ {
		temp = temp - int(math.Pow(float64(bf), float64(i)))
		height++
	}
	return &Tree{
		id:                myID,
		configurationSize: configurationSize,
		height:            height,
		branchFactor:      bf,
	}
}

// InitializeWithPIDs uses the map to initialize the position of replicas.
func (t *Tree) InitializeWithPIDs(ids []hotstuff.ID) error {
	if len(ids) != t.configurationSize {
		return fmt.Errorf("invalid number of replicas")
	}
	// check for duplicate IDs
	idToIndexMap := make(map[hotstuff.ID]int)
	for index, id := range ids {
		if _, ok := idToIndexMap[id]; !ok {
			idToIndexMap[id] = index
		} else {
			return fmt.Errorf("duplicate replica ID: %d", id)
		}
	}
	t.posToIDMapping = ids
	return nil
}

func (t *Tree) GetTreeHeight() int {
	return t.height
}

func (t *Tree) getPosition() (int, error) {
	pos := slices.Index(t.posToIDMapping, t.id)
	if pos == -1 {
		return 0, fmt.Errorf("replica not found")
	}
	return pos, nil
}

func (t *Tree) getReplicaPosition(replicaId hotstuff.ID) (int, error) {
	pos := slices.Index(t.posToIDMapping, replicaId)
	if pos == -1 {
		return 0, fmt.Errorf("replica not found")
	}
	return pos, nil
}

// GetParent fetches the ID of the parent, if root, returns itself.
func (t *Tree) GetParent() (hotstuff.ID, bool) {
	myPos, err := t.getPosition()
	if err != nil {
		return hotstuff.ID(0), false
	}
	if myPos == 0 {
		return t.id, false
	}
	return t.posToIDMapping[(myPos-1)/t.branchFactor], true
}

// GetChildren returns the children of the replicas, if any.
func (t *Tree) GetChildren() []hotstuff.ID {
	return t.GetChildrenOfNode(t.id)
}

func (t *Tree) isWithInIndex(position int) bool {
	return position < t.configurationSize
}

// IsRoot return true if the replica is at root of the tree.
func (t *Tree) IsRoot(nodeID hotstuff.ID) bool {
	pos, err := t.getReplicaPosition(nodeID)
	if err != nil {
		return false
	}
	return pos == 0
}

// GetChildrenOfNode returns the children of a specific replica.
func (t *Tree) GetChildrenOfNode(nodeID hotstuff.ID) []hotstuff.ID {
	children := make([]hotstuff.ID, 0)
	nodePos, err := t.getReplicaPosition(nodeID)
	if err != nil {
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

// getHeight returns the height of a given replica.
func (t *Tree) getHeight(nodeID hotstuff.ID) int {
	if t.IsRoot(nodeID) {
		return t.height
	}
	nodePos, err := t.getReplicaPosition(nodeID)
	if err != nil {
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

// GetPeers returns the peers of given ID, if any.
func (t *Tree) GetPeers(nodeID hotstuff.ID) []hotstuff.ID {
	peers := make([]hotstuff.ID, 0)
	if t.IsRoot(nodeID) {
		return peers
	}
	parent, ok := t.GetParent()
	if !ok {
		return peers
	}
	parentChildren := t.GetChildrenOfNode(parent)
	return parentChildren
}

// GetSubTreeNodes returns all the nodes of its subtree.
func (t *Tree) GetSubTreeNodes() []hotstuff.ID {
	nodeID := t.id
	subTreeNodes := make([]hotstuff.ID, 0)
	children := t.GetChildrenOfNode(nodeID)
	queue := make([]hotstuff.ID, 0)
	queue = append(queue, children...)
	subTreeNodes = append(subTreeNodes, children...)
	if len(children) == 0 {
		return subTreeNodes
	}
	for len(queue) > 0 {
		child := queue[0]
		queue = queue[1:]
		children := t.GetChildrenOfNode(child)
		subTreeNodes = append(subTreeNodes, children...)
		queue = append(queue, children...)
	}
	return subTreeNodes
}
