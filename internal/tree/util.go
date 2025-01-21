package tree

import (
	"reflect"

	"golang.org/x/exp/rand"
)

func Randomize(treePositions []uint32) {
	rnd := rand.New(rand.NewSource(rand.Uint64()))
	rnd.Shuffle(len(treePositions), reflect.Swapper(treePositions))
}
