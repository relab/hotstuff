package handel

import (
	"fmt"
	"math"

	"github.com/relab/hotstuff"
)

type partitioner struct {
	ids       []hotstuff.ID
	selfIndex int
}

func newPartitioner(selfID hotstuff.ID, allIDs []hotstuff.ID) (p partitioner) {
	p.ids = allIDs
	for i, v := range p.ids {
		if selfID == v {
			p.selfIndex = i
			break
		}
	}
	return p
}

func (p partitioner) rangeLevel(level int) (min int, max int) {
	maxLevel := int(math.Ceil(math.Log2(float64(len(p.ids)))))

	if level < 0 || level > maxLevel {
		panic(fmt.Sprintf("handel: invalid level %d", level))
	}

	max = len(p.ids) - 1

	for curLevel := maxLevel; curLevel >= level; curLevel-- {
		mid := (min + max) / 2

		if curLevel == level {
			// if we are at the target level, we want the half not containing the self
			if p.selfIndex > mid {
				max = mid
			} else {
				min = mid + 1
			}
		} else {
			// otherwise, we want the half containing the self
			if p.selfIndex > mid {
				min = mid + 1
			} else {
				max = mid
			}
		}
	}

	return min, max
}

func (p partitioner) partition(level int) []hotstuff.ID {
	min, max := p.rangeLevel(level)
	return p.ids[min : max+1]
}

func (p partitioner) size(level int) int {
	min, max := p.rangeLevel(level)
	return (max - min) + 1
}
