package kauri

import (
	"math"

	"github.com/relab/hotstuff"
)

// MaxChild determines the branching factor of the tree.
const MaxChild = 2

// TreeConfiguration is an abstraction for a tree communication model.
type TreeConfiguration interface {
	InitializeWithPIDs(ids map[hotstuff.ID]int)
	GetHeight() int
	GetChildren() []hotstuff.ID
	GetSubTreeNodes() []hotstuff.ID
	GetParent() (hotstuff.ID, bool)
	GetChildrenOfNode(nodeID hotstuff.ID) []hotstuff.ID
}

// FaultFreeTree implements a fault free tree configuration.
type FaultFreeTree struct {
	ID                  hotstuff.ID
	ConfigurationLength int
	height              int
	idToPosMapping      map[hotstuff.ID]int
	posToIDMapping      map[int]hotstuff.ID
}

// CreateTree Creates the tree configuration, currently only fault free tree configuration is supported.
func CreateTree(configurationLength int, myID hotstuff.ID) TreeConfiguration {

	if configurationLength <= 0 {
		return nil
	}
	temp := configurationLength
	temp = temp - 1 //root
	height := 1
	for i := 1; temp > 0; i++ {
		temp = temp - int(math.Pow(MaxChild, float64(i)))
		height++
	}
	return &FaultFreeTree{
		ID:                  myID,
		ConfigurationLength: configurationLength,
		height:              height,
	}
}

// InitializeWithPIDs uses the map to initialize the position of replicas.
func (t *FaultFreeTree) InitializeWithPIDs(ids map[hotstuff.ID]int) {
	t.idToPosMapping = ids
	t.posToIDMapping = make(map[int]hotstuff.ID)
	for ID, index := range ids {
		t.posToIDMapping[index] = ID
	}
}

// GetParent fetches the ID of the parent, if root, returns itself.
func (t *FaultFreeTree) GetParent() (hotstuff.ID, bool) {
	myPos := t.idToPosMapping[t.ID]
	if myPos == 0 {
		return t.ID, false
	}
	return t.posToIDMapping[(myPos-1)/MaxChild], true
}

// GetChildren returns the children of the replicas, if any.
func (t *FaultFreeTree) GetChildren() []hotstuff.ID {
	return t.GetChildrenOfNode(t.ID)
}

func (t *FaultFreeTree) isWithInIndex(position int) bool {
	return position < t.ConfigurationLength
}

// GetGrandParent returns grand parent of a replica, if not possible, return highest parent and false.
func (t *FaultFreeTree) GetGrandParent() (hotstuff.ID, bool) {
	parent, ok := t.GetParent()
	if !ok {
		return parent, false
	}
	parentPos := t.idToPosMapping[parent]
	if parentPos-1 < 0 {
		return parent, false
	}
	grandParentPos := (parentPos - 1) / MaxChild
	return t.posToIDMapping[grandParentPos], true
}

// IsRoot return true if the replica is at root of the tree.
func (t *FaultFreeTree) IsRoot(nodeID hotstuff.ID) bool {
	return t.idToPosMapping[nodeID] == 0
}

// GetChildrenOfNode returns the children of a specific replica.
func (t *FaultFreeTree) GetChildrenOfNode(nodeID hotstuff.ID) []hotstuff.ID {
	children := make([]hotstuff.ID, 0)
	nodePos := t.idToPosMapping[nodeID]
	for i := 1; i <= MaxChild; i++ {
		childPos := (MaxChild * nodePos) + i
		if t.isWithInIndex(childPos) {
			children = append(children, t.posToIDMapping[childPos])
		} else {
			break
		}
	}
	return children
}

// getHeight returns the height of a given replica.
func (t *FaultFreeTree) getHeight(nodeID hotstuff.ID) int {
	if t.IsRoot(nodeID) {
		return t.height
	}
	nodePos := t.idToPosMapping[nodeID]
	startLimit := 0
	endLimit := 0
	for i := 1; i < t.height; i++ {
		startLimit = startLimit + int(math.Pow(MaxChild, float64(i-1)))
		endLimit = endLimit + int(math.Pow(MaxChild, float64(i)))
		if nodePos >= startLimit && nodePos <= endLimit {
			return t.height - i
		}
	}
	return 0
}

// GetHeight returns the height of the replica
func (t *FaultFreeTree) GetHeight() int {
	return t.getHeight(t.ID)
}

// GetPeers returns the peers of given ID, if any.
func (t *FaultFreeTree) GetPeers(nodeID hotstuff.ID) []hotstuff.ID {
	peers := make([]hotstuff.ID, 0)
	if t.IsRoot(nodeID) {
		return peers
	}
	finalPeer := (MaxChild * t.getHeight(nodeID)) + 1
	startPeer := (MaxChild * (t.getHeight(nodeID) - 1)) + 1
	if finalPeer > t.ConfigurationLength {
		finalPeer = t.ConfigurationLength
	}
	for i := startPeer; i < finalPeer; i++ {
		peers = append(peers, t.posToIDMapping[i])
	}
	return peers
}

// GetSubTreeNodes returns all the nodes of its subtree.
func (t *FaultFreeTree) GetSubTreeNodes() []hotstuff.ID {
	nodeID := t.ID
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
