package handel

import (
	"context"
	"encoding/binary"
	"math/rand"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

const (
	levelActivateInterval = 50 * time.Millisecond
	disseminationPeriod   = 20 * time.Millisecond
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

func canMergeContributions(a, b consensus.IDSet) bool {
	canMerge := true

	a.RangeWhile(func(i hotstuff.ID) bool {
		b.RangeWhile(func(j hotstuff.ID) bool {
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

func (s *session) score(contribution contribution) int {
	if contribution.level < 1 || int(contribution.level) > s.h.maxLevel {
		// invalid level
		return 0
	}

	level := &s.levels[contribution.level]

	curBest := level.out

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

	// copy the set of participants and add all individual signatures
	finalParticipants := consensus.NewIDSet()
	contribution.signature.Participants().ForEach(finalParticipants.Add)

	indivAdded := 0

	for id := range level.individual {
		if !finalParticipants.Contains(id) {
			finalParticipants.Add(id)
			indivAdded++
		}
	}

	need := s.part.size(int(contribution.level))
	total := 0
	added := 0
	withIndiv := finalParticipants.Len()

	if curBest == nil {
		total = withIndiv
		added = total
	} else if canMergeContributions(contribution.signature.Participants(), curBest.Participants()) {
		curBest.Participants().ForEach(finalParticipants.Add)
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	} else {
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	}

	if added <= 0 {
		if contribution.isIndividual() {
			return 1
		}
		return 0
	}

	if total == need {
		return 1000000 - contribution.level*10 - indivAdded
	}

	return 100000 - contribution.level*100 + added*10 - indivAdded
}

// session
type session struct {
	h                  *Handel
	hash               consensus.Hash
	seed               int64
	part               partitioner
	levels             []level
	verificationQueue  verificationQueue
	contributions      chan contribution
	verifiedSignatures chan contribution
	newContributions   chan struct{}
	levelActivate      chan struct{}
	disseminate        chan struct{}
	done               chan consensus.ThresholdSignature
}

func (h *Handel) newSession(hash consensus.Hash, in consensus.ThresholdSignature) *session {
	s := &session{
		h:                  h,
		hash:               hash,
		seed:               h.mods.Options().SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:])),
		contributions:      make(chan contribution),
		verifiedSignatures: make(chan contribution),
		levelActivate:      make(chan struct{}),
		disseminate:        make(chan struct{}),
		done:               make(chan consensus.ThresholdSignature),
	}

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

	s.levels = make([]level, h.maxLevel+1)
	s.levels[0].individual[h.mods.ID()] = in
	s.levels[0].out = in
	s.levels[1].in = in

	// compute verification priority and contribution priority for all levels
	for i := 1; i <= h.maxLevel; i++ {
		s.levels[i].vp = verificationPriority(s.part.ids, s.seed, h.mods.ID(), i)
		s.levels[i].cp = contributionPriority(s.part.ids, s.seed, h.mods.ID(), i)
		s.levels[i].window = window{
			window:         h.mods.Configuration().Len(),
			max:            h.mods.Configuration().Len(),
			min:            2,
			increaseFactor: 2,
			decreaseFactor: 4,
		}
	}

	return s
}

type level struct {
	mut        sync.Mutex
	activated  bool
	done       bool
	vp         map[hotstuff.ID]int
	cp         map[hotstuff.ID]int
	in         consensus.ThresholdSignature
	out        consensus.ThresholdSignature
	individual map[hotstuff.ID]consensus.ThresholdSignature
	pending    []contribution
	window     window
	// startTime  time.Time
}

func (s *session) run(ctx context.Context) {
	levelTimerID := s.h.mods.EventLoop().AddTicker(levelActivateInterval, func(tick time.Time) (event interface{}) {
		s.levelActivate <- struct{}{}
		return nil
	})
	disseminationTimerID := s.h.mods.EventLoop().AddTicker(disseminationPeriod, func(tick time.Time) (event interface{}) {
		s.disseminate <- struct{}{}
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			s.h.mods.EventLoop().RemoveTicker(levelTimerID)
			s.h.mods.EventLoop().RemoveTicker(disseminationTimerID)
			break
		case c := <-s.verifiedSignatures:
			s.updateLevel(c)
		case c := <-s.contributions:
			s.insertPending(c)
		case <-s.levelActivate:
			s.activateLevel()
		case <-s.disseminate:
			s.sendContributions(ctx)
		}
	}
}

func (s *session) validLevel(l int) bool {
	return l > 0 && l <= s.h.maxLevel
}

func (s *session) insertPending(c contribution) {
	if !s.validLevel(c.level) {
		return
	}
	level := &s.levels[c.level]

	score := s.score(c)
	if score < 0 {
		return
	}

	level.mut.Lock()
	defer level.mut.Unlock()

	i := sort.Search(len(level.pending), func(i int) bool {
		other := level.pending[i]
		return level.vp[other.sender] < level.vp[c.sender]
	})

	level.pending = append(level.pending, contribution{})
	copy(level.pending[i+1:], level.pending[i:])
	level.pending[i] = c
}

func (s *session) updateLevel(c contribution) {
	level := &s.levels[c.level]
	level.mut.Lock()
	defer level.mut.Unlock()

	if c.isIndividual() {
		s.h.mods.Logger().Debugf("New individual signature from %d for level %d", c.sender, c.level)
		level.individual[c.sender] = c.signature
	}

	if c.signature.Participants().Len() > level.out.Participants().Len() {
		s.h.mods.Logger().Debugf("New aggregate signature for level %d with length %d", c.level, c.signature.Participants().Len())
		level.out = c.signature
	}
}

func (s *session) activateLevel() {
	for i := range s.levels {
		if !s.levels[i].activated {
			s.levels[i].activated = true
			break
		}
	}
}

func (s *session) sendContributions(ctx context.Context) {
	for i := 1; i <= s.h.maxLevel; i++ {
		prevLevel := &s.levels[i-1]
		level := &s.levels[i]
		level.mut.Lock()

		if !level.activated || level.done {
			level.mut.Unlock()
			continue
		}

		level.mut.Unlock()

		prevLevel.mut.Lock()
		in := prevLevel.out
		prevLevel.mut.Unlock()

		min, max := s.part.rangeLevel(i)
		part := s.part.ids[min : max+1]

		id := part[0]
		bestCP := level.cp[id]

		for _, i := range part {
			cp := level.cp[i]
			if cp < bestCP {
				bestCP = cp
				id = i
			}
		}

		// send in to id
		for _, node := range s.h.cfg.Nodes() {
			if node.ID() == uint32(id) {
				node.Contribute(ctx, &handelpb.Contribution{
					ID:         uint32(s.h.mods.ID()),
					Level:      uint32(i),
					Aggregate:  hotstuffpb.ThresholdSignatureToProto(in),
					Individual: hotstuffpb.ThresholdSignatureToProto(s.levels[0].out),
					Hash:       s.hash[:],
				})
			}
		}
	}
}

func (s *session) verifyContributions(ctx context.Context) {
	for ctx.Err() == nil {
		pending := 0

		for i := range s.levels {
			level := &s.levels[i]
			if level.activated && !level.done {
				pending += len(s.levels[i].pending)
				s.verifyContribution(ctx, &s.levels[i])
			}
		}

		if pending > 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.newContributions:
				continue
			}
		}
	}

}

func (s *session) verifyContribution(ctx context.Context, level *level) {
	level.mut.Lock()
	if len(level.pending) == 0 {
		level.mut.Unlock()
		return
	}

	c := level.pending[0]
	bestScore := s.score(c)
	var newPending []contribution

	for i := 1; i < len(level.pending); i++ {
		score := s.score(level.pending[i])
		if i < level.window.get() && score > bestScore {
			c = level.pending[i]
			bestScore = score
		} else if score > 0 {
			newPending = append(newPending, level.pending[i])
		}
	}

	if bestScore == 0 {
		return
	}

	level.pending = newPending

	// If the contribution is individual, we want to verify it separately
	if c.isIndividual() {
		if s.h.mods.Crypto().VerifyAggregateSignature(c.signature, s.hash) {
			s.verifiedSignatures <- c
		}
	}

	// combine with the current best signature, if possible
	agg := c.signature
	if canMergeContributions(agg.Participants(), level.out.Participants()) {
		agg = s.h.mods.Crypto().Combine(agg, level.out)
	}

	// add any individual signature, if possible
	for _, indiv := range level.individual {
		if canMergeContributions(agg.Participants(), indiv.Participants()) {
			agg = s.h.mods.Crypto().Combine(agg, indiv)
		}
	}

	level.mut.Unlock()

	if s.h.mods.Crypto().VerifyAggregateSignature(agg, s.hash) {
		level.mut.Lock()
		level.window.increase()
		level.mut.Unlock()

		s.verifiedSignatures <- contribution{
			level:     c.level,
			signature: agg,
		}

		if agg.Participants().Len() >= s.h.mods.Configuration().QuorumSize() {
			s.done <- agg
		}
	} else {
		level.mut.Lock()
		level.window.decrease()
		level.mut.Unlock()
	}
}

type window struct {
	window         int
	min            int
	max            int
	increaseFactor int
	decreaseFactor int
}

func (w *window) increase() {
	w.window *= w.increaseFactor
	if w.window > w.max {
		w.window = w.max
	}
}

func (w *window) decrease() {
	w.window /= w.decreaseFactor
	if w.window < w.min {
		w.window = w.min
	}
}

func (w window) get() int {
	return w.window
}
