package randel

import (
	"math"

	"github.com/relab/hotstuff"
)

type Level struct {
	posToLevelMapping []int
	idToPosMapping    map[hotstuff.ID]int
	posToIDMapping    map[int]hotstuff.ID
	subNodes          map[int][]int
	parentMapping     []int //Map node pos to its parent
	level1Peer        []int
	maxLevel          int
	ID                hotstuff.ID
}

func CreateLevelMapping(configurationLength int, myID hotstuff.ID) *Level {
	maxLevel := int(math.Ceil(math.Log2(float64(configurationLength))))
	posToLevelMapping := make([]int, configurationLength)
	subNodes := make(map[int][]int)
	level1Peer := make([]int, configurationLength)
	parentMapping := make([]int, configurationLength)
	posToLevelMapping[0] = maxLevel
	level1Nodes := make([]int, 0)
	for i := 1; i < configurationLength; i += 2 {
		level1Nodes = append(level1Nodes, i)
		posToLevelMapping[i] = 1
		subNodes[i] = []int{i - 1}
		level1Peer[i-1] = i
	}
	prevLevelNodes := level1Nodes
	for i := 2; i < maxLevel; i = i + 1 {
		tempNodes := make([]int, 0)
		for index := 0; index < len(prevLevelNodes); index += 2 {
			currentPos := (prevLevelNodes[index] + prevLevelNodes[index+1]) / 2
			subNodes[currentPos] = []int{prevLevelNodes[index], prevLevelNodes[index+1]}
			posToLevelMapping[currentPos] = i
			parentMapping[prevLevelNodes[index]] = currentPos
			parentMapping[prevLevelNodes[index+1]] = currentPos
			tempNodes = append(tempNodes, currentPos)
		}
		prevLevelNodes = tempNodes
	}
	subNodes[0] = []int{prevLevelNodes[0], prevLevelNodes[1]}
	parentMapping[prevLevelNodes[0]] = 0
	parentMapping[prevLevelNodes[1]] = 0
	return &Level{
		posToLevelMapping: posToLevelMapping,
		subNodes:          subNodes,
		level1Peer:        level1Peer,
		parentMapping:     parentMapping,
		ID:                myID,
		maxLevel:          maxLevel,
	}
}

func (l *Level) InitializeWithPIDs(posMapping map[hotstuff.ID]int) {
	l.idToPosMapping = posMapping
	l.posToIDMapping = make(map[int]hotstuff.ID)
	for ID, index := range posMapping {
		l.posToIDMapping[index] = ID
	}
}

func (l *Level) GetParent() hotstuff.ID {
	if l.GetLevel() == l.maxLevel {
		return l.ID
	}
	myPosition := l.idToPosMapping[l.ID]
	parentPos := l.parentMapping[myPosition]
	return l.getIDForPos(parentPos)
}

func (l *Level) GetLevel() int {
	myPosition := l.idToPosMapping[l.ID]
	return l.posToLevelMapping[myPosition]
}
func (l *Level) getIDForPos(position int) hotstuff.ID {
	return l.posToIDMapping[position]
}

func (l *Level) GetChildren() []hotstuff.ID {
	myPosition := l.idToPosMapping[l.ID]
	ids := make([]hotstuff.ID, 0)
	for _, pos := range l.subNodes[myPosition] {
		ids = append(ids, l.getIDForPos(pos))
	}
	return ids
}

func (l *Level) GetZeroLevelReplicas() []hotstuff.ID {
	ids := make([]hotstuff.ID, 0)
	for index, level := range l.posToLevelMapping {
		if level == 0 {
			ids = append(ids, l.getIDForPos(index))
		}
	}
	return ids
}

func (l *Level) GetGrandParent() hotstuff.ID {
	if l.GetLevel() == l.maxLevel {
		return l.ID
	}
	myPosition := l.idToPosMapping[l.ID]
	parentPos := l.parentMapping[myPosition]
	grandParentPos := l.parentMapping[parentPos]
	return l.getIDForPos(grandParentPos)
}

func (l *Level) GetLevel1Peer() hotstuff.ID {
	myPosition := l.idToPosMapping[l.ID]
	level1PeerPos := myPosition + 1
	return l.getIDForPos(level1PeerPos)
}
