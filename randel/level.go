package randel

import (
	"math"
	"math/rand"
	"reflect"
	"sort"

	"github.com/relab/hotstuff"
)

type Level struct {
	posMapping   map[int]hotstuff.ID
	levelMapping map[hotstuff.ID]int
	subNodes     map[hotstuff.ID][]hotstuff.ID
}

func (r *Randel) assignLevel() *Level {

	seed := r.mods.Options().SharedRandomSeed()
	totalNodes := r.mods.Configuration().Len()
	ids := make([]hotstuff.ID, 0, totalNodes)
	for id := range r.mods.Configuration().Replicas() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	maxLevel := int(math.Ceil(math.Log2(float64(len(ids)))))
	// Shuffle the list of IDs using the shared random seed + the first 8 bytes of the hash.
	rnd := rand.New(rand.NewSource(seed))
	rnd.Shuffle(len(ids), reflect.Swapper(ids))
	posMapping := make(map[int]hotstuff.ID)
	levelMapping := make(map[hotstuff.ID]int)
	for index, id := range ids {
		posMapping[index] = id
		levelMapping[id] = getLevelForIndex(index, maxLevel, totalNodes)
	}
	subNodes := assignSubNodes(posMapping, levelMapping)
	return &Level{
		posMapping:   posMapping,
		levelMapping: levelMapping,
		subNodes:     subNodes,
	}
}

func (l *Level) getAllSubNodes(id hotstuff.ID) []hotstuff.ID {
	return l.subNodes[id]
}

func getNodeIDsForLevel(levelMapping map[hotstuff.ID]int, level int) []hotstuff.ID {
	levelNodes := make([]hotstuff.ID, 0)
	for id, l := range levelMapping {
		if l == level {
			levelNodes = append(levelNodes, id)
		}
	}
	sort.Slice(levelNodes, func(i, j int) bool {
		return levelNodes[i] < levelNodes[j]
	})
	return levelNodes
}

func assignSubNodes(posMapping map[int]hotstuff.ID, levelMapping map[hotstuff.ID]int) map[hotstuff.ID][]hotstuff.ID {
	subNodes := make(map[hotstuff.ID][]hotstuff.ID)
	zeroProcess := posMapping[0]
	highLevel := levelMapping[zeroProcess]
	prevNodes := make([]hotstuff.ID, 0)
	prevNodes = append(prevNodes, zeroProcess)
	for highLevel > 1 {
		nextLowLevelNodes := getNodeIDsForLevel(levelMapping, highLevel-1)
		nextLevelAdded := make([]hotstuff.ID, 0)
		tempPrevNodes := make([]hotstuff.ID, 0)
		index := 0
		for _, tempID := range prevNodes {
			nextLevelAdded = append(nextLevelAdded, nextLowLevelNodes[index])
			nextLevelAdded = append(nextLevelAdded, nextLowLevelNodes[index+1])
			subNodes[tempID] = nextLevelAdded
			index = index + 2
			tempPrevNodes = append(tempPrevNodes, nextLevelAdded...)
			nextLevelAdded = make([]hotstuff.ID, 0)
		}
		prevNodes = tempPrevNodes
		highLevel -= 1
	}
	level1Nodes := getNodeIDsForLevel(levelMapping, 1)
	for _, nodeID := range level1Nodes {
		pos := getPosForNode(nodeID, posMapping)
		subNodes[nodeID] = []hotstuff.ID{posMapping[pos-1]}
	}
	return subNodes
}

func getPosForNode(nodeID hotstuff.ID, posMapping map[int]hotstuff.ID) int {
	for pos, tempNodeID := range posMapping {
		if tempNodeID == nodeID {
			return pos
		}
	}
	return 0
}

func getLevelForIndex(index int, maxLevel int, totalNodes int) int {

	if index == 0 {
		return maxLevel
	}
	startSequence := make([]int, 0)
	for i := 1; i < totalNodes; i += 2 {
		startSequence = append(startSequence, i)
	}
	for level := 1; level <= maxLevel; level += 1 {
		if isPresentInCurrentLevel(level, startSequence, index) {
			return level
		}
	}
	return 0
}

func isPresentInCurrentLevel(level int, startSequence []int, element int) bool {
	for _, ele := range startSequence {
		levelElement := ele * level
		if levelElement == element {
			return true
		}
	}
	return false
}

func (l *Level) getLevel(id hotstuff.ID) int {
	if len(l.getAllSubNodes(id)) == 0 {
		return 0
	}
	return l.levelMapping[id]
}
