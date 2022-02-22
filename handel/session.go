package handel

import (
	"context"
	"encoding/binary"
	"math/rand"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/relab/gorums"
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

func canMergeContributions(a, b consensus.ThresholdSignature) bool {
	canMerge := true

	if a == nil || b == nil {
		return false
	}

	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		b.Participants().RangeWhile(func(j hotstuff.ID) bool {
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

	need := s.part.size(int(contribution.level))
	if contribution.level == s.h.maxLevel {
		need = s.h.mods.Configuration().QuorumSize()
	}

	curBest := level.outgoing

	if curBest != nil && curBest.Participants().Len() >= need {
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

	total := 0
	added := 0
	withIndiv := finalParticipants.Len()

	if curBest == nil {
		total = withIndiv
		added = total
	} else if canMergeContributions(contribution.signature, curBest) {
		curBest.Participants().ForEach(finalParticipants.Add)
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	} else {
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	}

	s.h.mods.Logger().Debugf("level: %d, need: %d, added: %d, total: %d", contribution.level, need, added, total)

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
	h                *Handel
	hash             consensus.Hash
	seed             int64
	part             partitioner
	levels           []level
	allActive        bool
	newContributions chan struct{}
}

func (h *Handel) newSession(hash consensus.Hash, in consensus.ThresholdSignature) *session {
	s := &session{
		h:                h,
		hash:             hash,
		seed:             h.mods.Options().SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:])),
		newContributions: make(chan struct{}),
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

	h.mods.Logger().Debugf("Handel session ids: %v", ids)

	s.part = newPartitioner(h.mods.ID(), ids)

	s.levels = make([]level, h.maxLevel+1)
	for i := range s.levels {
		s.levels[i] = s.newLevel(i)

		min, max := s.part.rangeLevel(i)
		h.mods.Logger().Debugf("level %d: %v", i, s.part.ids[min:max+1])
	}

	s.levels[0].individual[h.mods.ID()] = in
	s.levels[0].outgoing = in

	return s
}

func (s *session) newLevel(i int) level {
	return level{
		vp:         verificationPriority(s.part.ids, s.seed, s.h.mods.ID(), i),
		cp:         contributionPriority(s.part.ids, s.seed, s.h.mods.ID(), i),
		individual: make(map[hotstuff.ID]consensus.ThresholdSignature),
		window: window{
			window:         s.h.mods.Configuration().Len(),
			max:            s.h.mods.Configuration().Len(),
			min:            2,
			increaseFactor: 2,
			decreaseFactor: 4,
		},
	}
}

type level struct {
	activated  bool
	vp         map[hotstuff.ID]int
	cp         map[hotstuff.ID]int
	incoming   consensus.ThresholdSignature
	outgoing   consensus.ThresholdSignature
	individual map[hotstuff.ID]consensus.ThresholdSignature
	pending    []contribution
	pendingMut sync.Mutex
	window     window
	// startTime  time.Time
}

func (s *session) handleContribution(c contribution) {
	if c.verified {
		s.updateIncoming(c)
	} else {
		s.insertPending(c)
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

	level.pendingMut.Lock()
	defer level.pendingMut.Unlock()

	i := sort.Search(len(level.pending), func(i int) bool {
		other := level.pending[i]
		return level.vp[other.sender] < level.vp[c.sender]
	})

	level.pending = append(level.pending, contribution{})
	copy(level.pending[i+1:], level.pending[i:])
	level.pending[i] = c

	s.h.mods.Logger().Debugf("pending contribution at level %d with score %d from sender %d", c.level, score, c.sender)

	// notify verification goroutine
	select {
	case s.newContributions <- struct{}{}:
	default:
	}
}

func (s *session) updateIncoming(c contribution) {
	level := &s.levels[c.level]

	if c.isIndividual() {
		s.h.mods.Logger().Debugf("New individual signature from %d for level %d", c.sender, c.level)
		level.individual[c.sender] = c.signature
	}

	if level.incoming == nil || c.signature.Participants().Len() > level.incoming.Participants().Len() {
		s.h.mods.Logger().Debugf("New incoming aggregate signature for level %d with length %d", c.level, c.signature.Participants().Len())
		level.incoming = c.signature
	}

	for i := c.level; i <= s.h.maxLevel-1; i++ {
		level := &s.levels[i]
		if canMergeContributions(level.incoming, level.outgoing) {
			agg := s.h.mods.Crypto().Combine(level.incoming, level.outgoing)
			s.updateOutgoing(contribution{
				hash:      c.hash,
				level:     c.level + 1,
				signature: agg,
				verified:  true,
			})
		} else {
			s.updateOutgoing(contribution{
				hash:      c.hash,
				level:     c.level + 1,
				signature: level.incoming,
				verified:  true,
			})
		}
	}

	maxLevel := &s.levels[s.h.maxLevel]
	if canMergeContributions(maxLevel.incoming, maxLevel.outgoing) {
		if maxLevel.incoming.Participants().Len()+maxLevel.outgoing.Participants().Len() >= s.h.mods.Configuration().QuorumSize() {
			s.h.mods.Logger().Panicf("Done with session: %.8s", s.hash)

			// send another round of messages before we quit this session.
			s.h.mods.EventLoop().DelayUntil(disseminateEvent{}, sessionDoneEvent{s.hash})

			s.h.mods.EventLoop().AddEvent(consensus.NewViewMsg{
				SyncInfo: consensus.NewSyncInfo().WithQC(consensus.NewQuorumCert(
					s.h.mods.Crypto().Combine(maxLevel.incoming, maxLevel.outgoing),
					s.h.mods.Synchronizer().View(),
					s.hash,
				)),
			})
		}
	}

}

func (s *session) updateOutgoing(c contribution) {
	level := &s.levels[c.level]

	if level.outgoing == nil || c.signature.Participants().Len() > level.outgoing.Participants().Len() {
		s.h.mods.Logger().Debugf("New outgoing aggregate signature for level %d with length %d", c.level, c.signature.Participants().Len())
		level.outgoing = c.signature
	}
}

func (s *session) activateLevel() {
	if s.allActive {
		return
	}

	for i := range s.levels {
		if !s.levels[i].activated {
			s.h.mods.Logger().Debugf("level %d activated", i)
			s.levels[i].activated = true
			return
		}
	}

	s.allActive = true
}

func (s *session) sendContributions(ctx context.Context) {
	for i := 1; i <= s.h.maxLevel; i++ {
		prevLevel := &s.levels[i-1]
		level := &s.levels[i]

		if !level.activated {
			continue
		}

		in := prevLevel.outgoing

		if in == nil {
			continue
		}

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
					Individual: hotstuffpb.ThresholdSignatureToProto(s.levels[0].outgoing),
					Hash:       s.hash[:],
				}, gorums.WithNoSendWaiting())
			}
		}

		// ensure we don't send to the same node each time
		level.cp[id] += len(s.part.ids)
	}
}

func (s *session) verifyContributions(ctx context.Context) {
	for ctx.Err() == nil {
		pending := 0

		for i := 1; i <= s.h.maxLevel; i++ {
			level := &s.levels[i]
			level.pendingMut.Lock()
			if level.activated {
				l := len(level.pending)
				pending += l
				level.pendingMut.Unlock()
				if l > 0 {
					s.verifyContribution(level)
				}
			} else {
				level.pendingMut.Unlock()
			}
		}

		if pending > 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.newContributions:
			}
		}
	}

}

func (s *session) verifyContribution(level *level) {
	level.pendingMut.Lock()
	if len(level.pending) == 0 {
		level.pendingMut.Unlock()
		return
	}

	c := level.pending[0]
	bestScore := s.score(c)
	var newPending []contribution

	for i := 1; i < len(level.pending); i++ {
		score := s.score(level.pending[i])
		if i < level.window.get() && score > bestScore {
			newPending = append(newPending, c)
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

	// combine with the current best signature, if possible
	agg := c.signature
	if level.outgoing != nil && canMergeContributions(agg, level.outgoing) {
		agg = s.h.mods.Crypto().Combine(agg, level.outgoing)
	}

	// add any individual signature, if possible
	for _, indiv := range level.individual {
		if canMergeContributions(agg, indiv) {
			agg = s.h.mods.Crypto().Combine(agg, indiv)
		}
	}

	level.pendingMut.Unlock()

	indivVerified := false
	// If the contribution is individual, we want to verify it separately
	if c.isIndividual() {
		if s.h.mods.Crypto().VerifyAggregateSignature(c.signature, s.hash) {
			indivVerified = true
			s.h.mods.EventLoop().AddEvent(contribution{
				hash:      c.hash,
				sender:    c.sender,
				level:     c.level,
				signature: c.signature,
				verified:  true,
			})
		}
	}

	aggVerified := false
	if s.h.mods.Crypto().VerifyAggregateSignature(agg, s.hash) {
		aggVerified = true
		s.h.mods.EventLoop().AddEvent(contribution{
			hash:      c.hash,
			sender:    c.sender,
			level:     c.level,
			signature: agg,
			verified:  true,
		})
	}

	indivOk := (!c.isIndividual() || indivVerified)

	if indivOk && aggVerified {
		s.h.mods.Logger().Debug("window increased")
		level.window.increase()
	} else {
		s.h.mods.Logger().Debugf("window decreased (indiv: %v, agg: %v)", indivOk, aggVerified)
		level.window.decrease()
	}
}

type window struct {
	mut            sync.Mutex
	window         int
	min            int
	max            int
	increaseFactor int
	decreaseFactor int
}

func (w *window) increase() {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.window *= w.increaseFactor
	if w.window > w.max {
		w.window = w.max
	}
}

func (w *window) decrease() {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.window /= w.decreaseFactor
	if w.window < w.min {
		w.window = w.min
	}
}

func (w *window) get() int {
	w.mut.Lock()
	defer w.mut.Unlock()

	return w.window
}
