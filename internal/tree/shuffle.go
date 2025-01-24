package tree

import (
	"math/rand/v2"
)

var rnd = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

func Shuffle(treePositions []uint32) {
	rnd.Shuffle(len(treePositions), func(i, j int) {
		treePositions[i], treePositions[j] = treePositions[j], treePositions[i]
	})
}
