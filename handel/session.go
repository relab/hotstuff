package handel

import (
	"encoding/binary"
	"math/rand"
	"reflect"
	"sort"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
)

// verificationPriority returns a pseudorandom permutation of 0..n-1 where n is the number of nodes,
// seed is a random number, self is the id of the local node, and level is the verification level.
// The verification priority (vp) is used to determine whose contributions can be verified.
func verificationPriority(ids []hotstuff.ID, seed int64, self hotstuff.ID, level int) (vp map[hotstuff.ID]int) {
	vp = make(map[hotstuff.ID]int)

	// create a slice of numbers [0, n-1) and shuffle it
	numbers := make([]int, len(ids)-1)
	for i := range numbers {
		numbers[i] = i
	}
	rnd := rand.New(rand.NewSource(seed + int64(level)))
	rnd.Shuffle(len(numbers), reflect.Swapper(numbers))

	// assign ids to numbers
	i := 0
	for _, id := range ids {
		if id == self {
			continue
		}
		vp[id] = numbers[i]
		i++
	}

	return vp
}

// contributionPriority returns a map of each remote node's verification priority for the local node.
// The contribution priority (cp) is used to determine which nodes should be contacted first.
func contributionPriority(ids []hotstuff.ID, seed int64, self hotstuff.ID, level int) (cp map[hotstuff.ID]int) {
	cp = make(map[hotstuff.ID]int)

	for _, id := range ids {
		if id == self {
			continue
		}
		vp := verificationPriority(ids, seed, id, level)
		cp[id] = vp[self]
	}

	return cp
}

func canMergeContributions(a, b consensus.ThresholdSignature) bool {
	setA := a.Participants()
	setB := b.Participants()
	canMerge := true

	setA.RangeWhile(func(i hotstuff.ID) bool {
		setB.RangeWhile(func(j hotstuff.ID) bool {
			// cannot merge a and b if they both contain a contribution from the same ID.
			if i == j {
				canMerge = false
			}
			return canMerge
		})
		return canMerge
	})

	return canMerge
}

func (s *session) score(contribution *handelpb.Contribution) int {
	if contribution.GetLevel() < 1 || int(contribution.GetLevel()) > s.h.maxLevel {
		// invalid level
		return 0
	}

	curBest := s.levels[contribution.GetLevel()].out

	if curBest != nil && curBest.Participants().Len() >= s.h.mods.Configuration().QuorumSize() {
		// level is completed, no need for this signature.
		return 0
	}

	// scoring priority (based on reference implementation):
	// 1. Completed level (prioritize first levels)
	// 2. Added value to level (prioritize old levels)
	// 3. Individual signatures
	//
	// Signatures that add no value to a level are simply ignored.

	need := s.part.size(int(contribution.GetLevel()))

}

// session
type session struct {
	h      *Handel
	seed   int64
	part   partitioner
	levels []level
}

func (h *Handel) newSession(hash consensus.Hash) *session {
	s := &session{}
	s.h = h
	s.seed = h.mods.Options().SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:]))

	// Get a sorted list of IDs for all replicas.
	// The configuration should also contain our own ID.
	ids := make([]hotstuff.ID, 0, h.mods.Configuration().Len())
	for id := range h.mods.Configuration().Replicas() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// Shuffle the list of IDs using the shared random seed + the first 8 bytes of the hash.
	rnd := rand.New(rand.NewSource(s.seed))
	rnd.Shuffle(len(ids), reflect.Swapper(ids))

	h.mods.Logger().Debug("Handel session ids: %v", ids)

	s.part = newPartitioner(h.mods.ID(), ids)
	// compute verification priority and

	return s
}
