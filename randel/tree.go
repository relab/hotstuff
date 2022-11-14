package randel

import (
	"math"

	"github.com/relab/hotstuff"
)

const MAX_DEPTH = 3
const MAX_CHILD = 4

type TreeConfiguration struct {
	ID                  hotstuff.ID
	ConfigurationLength int
	height              int
	idToPosMapping      map[hotstuff.ID]int
	posToIDMapping      map[int]hotstuff.ID
}

func CreateTree(configurationLength int, myID hotstuff.ID) *TreeConfiguration {

	if configurationLength <= 0 {
		return nil
	}
	temp := configurationLength
	temp = temp - 1 //root
	height := 1
	for i := 1; temp > 0; i++ {
		temp = temp - int(math.Pow(MAX_CHILD, float64(i)))
		height++
	}
	return &TreeConfiguration{
		ID:                  myID,
		ConfigurationLength: configurationLength,
		height:              height,
	}
}

func (t *TreeConfiguration) InitializeWithPIDs(ids map[hotstuff.ID]int) {
	t.idToPosMapping = ids
	t.posToIDMapping = make(map[int]hotstuff.ID)
	for ID, index := range ids {
		t.posToIDMapping[index] = ID
	}
}

func (t *TreeConfiguration) GetParent() (hotstuff.ID, bool) {
	myPos := t.idToPosMapping[t.ID]
	if myPos == 0 {
		return t.ID, false
	}
	return t.posToIDMapping[(myPos-1)/MAX_CHILD], true
}

func (t *TreeConfiguration) GetChildren() []hotstuff.ID {
	return t.GetChildrenOfNode(t.ID)
}

func (t *TreeConfiguration) isWithInIndex(position int) bool {
	if position < t.ConfigurationLength {
		return true
	} else {
		return false
	}
}
func (t *TreeConfiguration) GetGrandParent() (hotstuff.ID, bool) {
	parent, ok := t.GetParent()
	if !ok {
		return parent, false
	}
	parentPos := t.idToPosMapping[parent]
	if parentPos-1 < 0 {
		return parent, false
	}
	grandParentPos := (parentPos - 1) / MAX_CHILD
	return t.posToIDMapping[grandParentPos], true
}

func (t *TreeConfiguration) IsRoot(nodeID hotstuff.ID) bool {
	return t.idToPosMapping[nodeID] == 0
}

func (t *TreeConfiguration) GetChildrenOfNode(nodeID hotstuff.ID) []hotstuff.ID {
	children := make([]hotstuff.ID, 0)
	nodePos := t.idToPosMapping[nodeID]
	for i := 1; i <= MAX_CHILD; i++ {
		childPos := (MAX_CHILD * nodePos) + i
		if t.isWithInIndex(childPos) {
			children = append(children, t.posToIDMapping[childPos])
		} else {
			break
		}
	}
	return children
}

func (t *TreeConfiguration) GetHeight(nodeID hotstuff.ID) int {
	if t.IsRoot(nodeID) {
		return t.height
	}
	nodePos := t.idToPosMapping[nodeID]
	startLimit := 0
	endLimit := 0
	for i := 1; i < t.height; i++ {
		startLimit = startLimit + int(math.Pow(MAX_CHILD, float64(i-1)))
		endLimit = endLimit + int(math.Pow(MAX_CHILD, float64(i)))
		if nodePos >= startLimit && nodePos <= endLimit {
			return t.height - i
		}
	}
	return 0
}

func (t *TreeConfiguration) GetPeers(nodeID hotstuff.ID) []hotstuff.ID {
	peers := make([]hotstuff.ID, 0)
	if t.IsRoot(nodeID) {
		return peers
	}
	finalPeer := (MAX_CHILD * t.GetHeight(nodeID)) + 1
	startPeer := (MAX_CHILD * (t.GetHeight(nodeID) - 1)) + 1
	if finalPeer > t.ConfigurationLength {
		finalPeer = t.ConfigurationLength
	}
	for i := startPeer; i < finalPeer; i++ {
		peers = append(peers, t.posToIDMapping[i])
	}
	return peers
}

func (t *TreeConfiguration) GetSubTreeNodes(nodeID hotstuff.ID) []hotstuff.ID {
	subTreeNodes := make([]hotstuff.ID, 0)
	children := t.GetChildrenOfNode(nodeID)
	queue := make([]hotstuff.ID, 0)
	queue = append(queue, children...)
	subTreeNodes = append(subTreeNodes, children...)
	if len(children) == 0 {
		return subTreeNodes
	} else {
		for len(queue) > 0 {
			child := queue[0]
			queue = queue[1:]
			children := t.GetChildrenOfNode(child)
			subTreeNodes = append(subTreeNodes, children...)
			queue = append(queue, children...)
		}
	}
	return subTreeNodes
}
