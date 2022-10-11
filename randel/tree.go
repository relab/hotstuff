package randel

import (
	"github.com/relab/hotstuff"
)

const MAX_DEPTH = 3
const MAX_CHILD = 3

type TreeConfiguration struct {
	ID                  hotstuff.ID
	ConfigurationLength int
	idToPosMapping      map[hotstuff.ID]int
	posToIDMapping      map[int]hotstuff.ID
}

func CreateTree(configurationLength int, myID hotstuff.ID) *TreeConfiguration {

	return &TreeConfiguration{
		ID:                  myID,
		ConfigurationLength: configurationLength,
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
	children := make([]hotstuff.ID, 0)
	myPos := t.idToPosMapping[t.ID]
	for i := 1; i <= MAX_CHILD; i++ {
		childPos := (MAX_CHILD * myPos) + i
		if t.isWithInIndex(childPos) {
			children = append(children, t.posToIDMapping[childPos])
		} else {
			break
		}
	}
	return children
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

func (t *TreeConfiguration) IsRoot() bool {
	return t.idToPosMapping[t.ID] == 0
}
