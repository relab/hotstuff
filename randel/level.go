package randel

import (
	"encoding/binary"
	"math"
	"math/rand"
	"reflect"
	"sort"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type Level struct {
	posMapping   map[int]hotstuff.ID
	levelMapping map[hotstuff.ID]int
	seed         int64
}

func (r *Randel) assignLevel(hash consensus.Hash) *Level {

	seed := r.mods.Options().SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:]))
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
	return &Level{
		posMapping:   posMapping,
		seed:         seed,
		levelMapping: levelMapping,
	}
}

func (l *Level) getAllSubNodes(id hotstuff.ID) []hotstuff.ID {
	pos := 0
	for tempPos, tempID := range l.posMapping {
		if tempID == id {
			pos = tempPos
		}
	}
	subNodes := make([]hotstuff.ID, 0)
	level := l.levelMapping[id]
	prevLevel := 0
	for tempPos := 1; tempPos < len(l.posMapping); tempPos++ {
		if tempPos > pos && l.levelMapping[l.posMapping[tempPos]] > prevLevel &&
			l.levelMapping[l.posMapping[tempPos]] < level {
			subNodes = append(subNodes, l.posMapping[tempPos])
			prevLevel = l.levelMapping[l.posMapping[tempPos]]
		}
	}
	return subNodes
}

func getLevelForIndex(index int, maxLevel int, totalNodes int) int {

	if index == 0 {
		return maxLevel
	}
	startSequence := make([]int, 0, totalNodes/2+(totalNodes%2))
	for i := 1; i < totalNodes; {
		startSequence = append(startSequence, i)
		i += 2
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

func (l *Level) getAllNodesForLevel(level int) []hotstuff.ID {
	nodes := make([]hotstuff.ID, 0)
	for id, currentLevel := range l.levelMapping {
		if level == currentLevel {
			nodes = append(nodes, id)
		}
	}
	return nodes
}

func (l *Level) getSubNodesForNode(level int, myID hotstuff.ID) []hotstuff.ID {
	retNodes := make([]hotstuff.ID, 0)
	subNodes := l.getAllSubNodes(myID)
	for id, currentLevel := range l.levelMapping {
		if currentLevel == level && isPresentInSlice(id, subNodes) {
			retNodes = append(retNodes, id)
		}
	}
	return retNodes
}

func isPresentInSlice(element hotstuff.ID, idSlice []hotstuff.ID) bool {
	for _, id := range idSlice {
		if element == id {
			return true
		}
	}
	return false
}

func (l *Level) getLevel(id hotstuff.ID) int {
	return l.levelMapping[id]
}
