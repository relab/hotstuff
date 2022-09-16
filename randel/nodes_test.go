package randel

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff"
)

func TestAssignSubNodes(t *testing.T) {

	testLengths := []int{8, 16, 32}
	expectedLevelResult := map[int]map[int]int{
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
			8:  0,
			9:  1,
			10: 2,
			11: 1,
			12: 3,
			13: 1,
			14: 2,
			15: 1,
		},
		32: {
			0:  5,
			1:  1,
			2:  2,
			3:  1,
			4:  3,
			5:  1,
			6:  2,
			7:  1,
			8:  4,
			9:  1,
			10: 2,
			11: 1,
			12: 3,
			13: 1,
			14: 2,
			15: 1,
			16: 0,
			17: 1,
			18: 2,
			19: 1,
			20: 3,
			21: 1,
			22: 2,
			23: 1,
			24: 4,
			25: 1,
			26: 2,
			27: 1,
			28: 3,
			29: 1,
			30: 2,
			31: 1,
		},
	}
	expectedResult := map[int]map[hotstuff.ID][]hotstuff.ID{
		8: {
			hotstuff.ID(0): {hotstuff.ID(2), hotstuff.ID(6)},
			hotstuff.ID(1): {hotstuff.ID(0)},
			hotstuff.ID(2): {hotstuff.ID(1), hotstuff.ID(3)},
			hotstuff.ID(3): {hotstuff.ID(2)},
			hotstuff.ID(5): {hotstuff.ID(4)},
			hotstuff.ID(6): {hotstuff.ID(5), hotstuff.ID(7)},
			hotstuff.ID(7): {hotstuff.ID(6)},
			hotstuff.ID(4): {},
		},
		16: {
			hotstuff.ID(0):  {hotstuff.ID(4), hotstuff.ID(12)},
			hotstuff.ID(1):  {hotstuff.ID(0)},
			hotstuff.ID(2):  {hotstuff.ID(1), hotstuff.ID(3)},
			hotstuff.ID(3):  {hotstuff.ID(2)},
			hotstuff.ID(4):  {hotstuff.ID(2), hotstuff.ID(6)},
			hotstuff.ID(5):  {hotstuff.ID(4)},
			hotstuff.ID(6):  {hotstuff.ID(5), hotstuff.ID(7)},
			hotstuff.ID(7):  {hotstuff.ID(6)},
			hotstuff.ID(8):  {},
			hotstuff.ID(9):  {hotstuff.ID(8)},
			hotstuff.ID(10): {hotstuff.ID(9), hotstuff.ID(11)},
			hotstuff.ID(11): {hotstuff.ID(10)},
			hotstuff.ID(12): {hotstuff.ID(10), hotstuff.ID(14)},
			hotstuff.ID(13): {hotstuff.ID(12)},
			hotstuff.ID(14): {hotstuff.ID(13), hotstuff.ID(15)},
			hotstuff.ID(15): {hotstuff.ID(14)},
		},
		32: {
			hotstuff.ID(0):  {hotstuff.ID(8), hotstuff.ID(24)},
			hotstuff.ID(1):  {hotstuff.ID(0)},
			hotstuff.ID(2):  {hotstuff.ID(1), hotstuff.ID(3)},
			hotstuff.ID(3):  {hotstuff.ID(2)},
			hotstuff.ID(4):  {hotstuff.ID(2), hotstuff.ID(6)},
			hotstuff.ID(5):  {hotstuff.ID(4)},
			hotstuff.ID(6):  {hotstuff.ID(5), hotstuff.ID(7)},
			hotstuff.ID(7):  {hotstuff.ID(6)},
			hotstuff.ID(8):  {hotstuff.ID(4), hotstuff.ID(12)},
			hotstuff.ID(9):  {hotstuff.ID(8)},
			hotstuff.ID(10): {hotstuff.ID(9), hotstuff.ID(11)},
			hotstuff.ID(11): {hotstuff.ID(10)},
			hotstuff.ID(12): {hotstuff.ID(10), hotstuff.ID(14)},
			hotstuff.ID(13): {hotstuff.ID(12)},
			hotstuff.ID(14): {hotstuff.ID(13), hotstuff.ID(15)},
			hotstuff.ID(15): {hotstuff.ID(14)},
			hotstuff.ID(16): {},
			hotstuff.ID(17): {hotstuff.ID(16)},
			hotstuff.ID(18): {hotstuff.ID(17), hotstuff.ID(19)},
			hotstuff.ID(19): {hotstuff.ID(18)},
			hotstuff.ID(20): {hotstuff.ID(18), hotstuff.ID(22)},
			hotstuff.ID(21): {hotstuff.ID(20)},
			hotstuff.ID(22): {hotstuff.ID(21), hotstuff.ID(23)},
			hotstuff.ID(23): {hotstuff.ID(22)},
			hotstuff.ID(24): {hotstuff.ID(20), hotstuff.ID(28)},
			hotstuff.ID(25): {hotstuff.ID(24)},
			hotstuff.ID(26): {hotstuff.ID(25), hotstuff.ID(27)},
			hotstuff.ID(27): {hotstuff.ID(26)},
			hotstuff.ID(28): {hotstuff.ID(26), hotstuff.ID(30)},
			hotstuff.ID(29): {hotstuff.ID(28)},
			hotstuff.ID(30): {hotstuff.ID(29), hotstuff.ID(31)},
			hotstuff.ID(31): {hotstuff.ID(30)},
		},
	}
	expectedParentResult := map[int]map[hotstuff.ID]hotstuff.ID{
		8: {
			hotstuff.ID(1): hotstuff.ID(2),
			hotstuff.ID(2): hotstuff.ID(0),
			hotstuff.ID(3): hotstuff.ID(2),
			hotstuff.ID(5): hotstuff.ID(6),
			hotstuff.ID(6): hotstuff.ID(0),
			hotstuff.ID(7): hotstuff.ID(6),
		},
		16: {
			hotstuff.ID(1):  hotstuff.ID(2),
			hotstuff.ID(2):  hotstuff.ID(4),
			hotstuff.ID(3):  hotstuff.ID(2),
			hotstuff.ID(4):  hotstuff.ID(0),
			hotstuff.ID(5):  hotstuff.ID(6),
			hotstuff.ID(6):  hotstuff.ID(4),
			hotstuff.ID(7):  hotstuff.ID(6),
			hotstuff.ID(8):  hotstuff.ID(0),
			hotstuff.ID(9):  hotstuff.ID(10),
			hotstuff.ID(10): hotstuff.ID(12),
			hotstuff.ID(11): hotstuff.ID(10),
			hotstuff.ID(12): hotstuff.ID(0),
			hotstuff.ID(13): hotstuff.ID(14),
			hotstuff.ID(14): hotstuff.ID(12),
			hotstuff.ID(15): hotstuff.ID(14),
		},
		32: {
			hotstuff.ID(0):  hotstuff.ID(0),
			hotstuff.ID(1):  hotstuff.ID(2),
			hotstuff.ID(2):  hotstuff.ID(4),
			hotstuff.ID(3):  hotstuff.ID(2),
			hotstuff.ID(4):  hotstuff.ID(8),
			hotstuff.ID(5):  hotstuff.ID(6),
			hotstuff.ID(6):  hotstuff.ID(4),
			hotstuff.ID(7):  hotstuff.ID(6),
			hotstuff.ID(8):  hotstuff.ID(0),
			hotstuff.ID(9):  hotstuff.ID(10),
			hotstuff.ID(10): hotstuff.ID(12),
			hotstuff.ID(11): hotstuff.ID(10),
			hotstuff.ID(12): hotstuff.ID(8),
			hotstuff.ID(13): hotstuff.ID(14),
			hotstuff.ID(14): hotstuff.ID(12),
			hotstuff.ID(15): hotstuff.ID(14),
			hotstuff.ID(16): hotstuff.ID(0),
			hotstuff.ID(17): hotstuff.ID(18),
			hotstuff.ID(18): hotstuff.ID(20),
			hotstuff.ID(19): hotstuff.ID(18),
			hotstuff.ID(20): hotstuff.ID(24),
			hotstuff.ID(21): hotstuff.ID(22),
			hotstuff.ID(22): hotstuff.ID(20),
			hotstuff.ID(23): hotstuff.ID(22),
			hotstuff.ID(24): hotstuff.ID(0),
			hotstuff.ID(25): hotstuff.ID(26),
			hotstuff.ID(26): hotstuff.ID(28),
			hotstuff.ID(27): hotstuff.ID(26),
			hotstuff.ID(28): hotstuff.ID(24),
			hotstuff.ID(29): hotstuff.ID(30),
			hotstuff.ID(30): hotstuff.ID(28),
			hotstuff.ID(31): hotstuff.ID(30),
		},
	}

	for _, length := range testLengths {

		posMapping := make(map[hotstuff.ID]int)
		for i := 0; i < length; i++ {
			posMapping[hotstuff.ID(i)] = i
		}

		for i := 0; i < length; i++ {
			level := CreateLevelMapping(length, hotstuff.ID(i))
			level.InitializeWithPIDs(posMapping)
			if diff := cmp.Diff(level.GetChildren(), expectedResult[length][hotstuff.ID(i)]); diff != "" {
				t.Errorf("error in GetChildren(%d) and Diff is %v, got %v and expected %v\n", i, diff,
					level.GetChildren(), expectedResult[length][hotstuff.ID(i)])
			}
			if level.GetLevel() != expectedLevelResult[length][i] {
				t.Errorf("Error in GetLevel(%d), expected %d and got %d\n", i, expectedLevelResult[length][i],
					level.GetLevel())
			}
			if level.GetParent() != expectedParentResult[length][hotstuff.ID(i)] {
				t.Errorf("Error in GetParent(%d) in length %d, expected %d and got %d\n", i, length, expectedParentResult[length][hotstuff.ID(i)],
					level.GetParent())
			}
		}

	}
}

func TestGrandParent(t *testing.T) {
	length := 32
	expectedGP := map[int]hotstuff.ID{
		0:  hotstuff.ID(0),
		1:  hotstuff.ID(4),
		2:  hotstuff.ID(8),
		3:  hotstuff.ID(4),
		4:  hotstuff.ID(0),
		5:  hotstuff.ID(4),
		6:  hotstuff.ID(8),
		7:  hotstuff.ID(4),
		8:  hotstuff.ID(0),
		9:  hotstuff.ID(12),
		10: hotstuff.ID(8),
		11: hotstuff.ID(12),
		12: hotstuff.ID(0),
		13: hotstuff.ID(12),
		14: hotstuff.ID(8),
		15: hotstuff.ID(12),
		16: hotstuff.ID(0),
		17: hotstuff.ID(20),
		18: hotstuff.ID(24),
		19: hotstuff.ID(20),
		20: hotstuff.ID(0),
		21: hotstuff.ID(20),
		22: hotstuff.ID(24),
		23: hotstuff.ID(20),
		24: hotstuff.ID(0),
		25: hotstuff.ID(28),
		26: hotstuff.ID(24),
		27: hotstuff.ID(28),
		28: hotstuff.ID(0),
		29: hotstuff.ID(28),
		30: hotstuff.ID(24),
		31: hotstuff.ID(28),
	}
	posMapping := make(map[hotstuff.ID]int)
	for i := 0; i < length; i++ {
		posMapping[hotstuff.ID(i)] = i
	}
	for i := 0; i < length; i++ {
		level := CreateLevelMapping(length, hotstuff.ID(i))
		level.InitializeWithPIDs(posMapping)
		if level.GetGrandParent() != expectedGP[i] {
			t.Errorf("error in GetGrandParent(%d) in length %d expected GP is %v and Got GP is %v\n",
				i, length, expectedGP[i], level.GetGrandParent())
		}
	}
}
