package randel

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff"
)

func TestAssignSubNodes(t *testing.T) {

	testLengths := []int{8}
	expectedResult := map[int]map[hotstuff.ID][]hotstuff.ID{
		8: {
			hotstuff.ID(0): {hotstuff.ID(2), hotstuff.ID(6)},
			hotstuff.ID(1): {hotstuff.ID(0)},
			hotstuff.ID(2): {hotstuff.ID(1), hotstuff.ID(3)},
			hotstuff.ID(3): {hotstuff.ID(2)},
			hotstuff.ID(5): {hotstuff.ID(4)},
			hotstuff.ID(6): {hotstuff.ID(5), hotstuff.ID(7)},
			hotstuff.ID(7): {hotstuff.ID(6)},
		},
		16: {
			hotstuff.ID(0):  {hotstuff.ID(4), hotstuff.ID(8)},
			hotstuff.ID(1):  {hotstuff.ID(0)},
			hotstuff.ID(2):  {hotstuff.ID(1), hotstuff.ID(3)},
			hotstuff.ID(3):  {hotstuff.ID(2)},
			hotstuff.ID(5):  {hotstuff.ID(4)},
			hotstuff.ID(6):  {hotstuff.ID(5), hotstuff.ID(7)},
			hotstuff.ID(7):  {hotstuff.ID(6)},
			hotstuff.ID(8):  {hotstuff.ID(10), hotstuff.ID(14)},
			hotstuff.ID(9):  {hotstuff.ID(8)},
			hotstuff.ID(10): {hotstuff.ID(9), hotstuff.ID(11)},
			hotstuff.ID(11): {hotstuff.ID(10)},
			hotstuff.ID(13): {hotstuff.ID(12)},
			hotstuff.ID(14): {hotstuff.ID(13), hotstuff.ID(14)},
			hotstuff.ID(15): {hotstuff.ID(14)},
		},
	}

	for _, length := range testLengths {
		posMapping := make(map[int]hotstuff.ID)
		levelMapping := make(map[hotstuff.ID]int)
		maxLevel := int(math.Ceil(math.Log2(float64(length))))
		ids := make([]hotstuff.ID, length)
		for i := 0; i < length; i++ {
			ids[i] = hotstuff.ID(i)
			posMapping[i] = hotstuff.ID(i)
			levelMapping[hotstuff.ID(i)] = getLevelForIndex(i, maxLevel, length)
		}
		gotResult := assignSubNodes(posMapping, levelMapping)
		if diff := cmp.Diff(gotResult, expectedResult[length]); diff != "" {
			t.Errorf("error in assignSubNodes diff is %v\n", gotResult)
			t.Errorf("got result is %v\n", gotResult)
			t.Errorf("expected result is %v\n", expectedResult)
		}
	}

}

func TestLevelForIndex(t *testing.T) {
	testLengths := []int{8}
	expectedResult := map[int]map[int]int{
		8: {
			0: 3,
			1: 1,
			2: 2,
			3: 1,
			4: 0,
			5: 1,
			6: 2,
			7: 1,
		},
		16: {
			0:  4,
			1:  1,
			2:  2,
			3:  1,
			4:  3,
			5:  1,
			6:  2,
			7:  1,
			8:  3,
			9:  1,
			10: 2,
			11: 1,
			12: 3,
			13: 1,
			14: 2,
			15: 1,
		},
	}
	for _, length := range testLengths {
		maxLevel := int(math.Ceil(math.Log2(float64(length))))

		for i := 0; i < length; i++ {
			gotLevel := getLevelForIndex(i, maxLevel, length)
			if gotLevel != expectedResult[length][i] {
				t.Fatalf("Expected level for index %d is %d, but got %d", i, expectedResult[i], gotLevel)
				t.Fail()
			}
		}
	}
}
