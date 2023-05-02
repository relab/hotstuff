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
	"github.com/relab/hotstuff/internal/proto/handelpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/synchronizer"
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

type session struct {
	// mutex is needed because pending and activeLevel.incoming
	// are accessed by verifyContributions in a separate goroutine

	mut sync.Mutex
	h   *Handel

	// static data
	hash hotstuff.Hash
	seed int64
	part partitioner

	// levels
	levels           []level
	activeLevelIndex int

	// verification
	window           window
	newContributions chan struct{}

	// timers
	levelActivateTimerID int
	disseminateTimerID   int
}

func (h *Handel) newSession(hash hotstuff.Hash, in hotstuff.QuorumSignature) *session {
	s := &session{
		h:    h,
		hash: hash,
		seed: h.opts.SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:])),

		window: window{
			window:         h.configuration.Len(),
			max:            h.configuration.Len(),
			min:            2,
			increaseFactor: 2,
			decreaseFactor: 4,
		},
		newContributions: make(chan struct{}),
	}

	// Get a sorted list of IDs for all replicas.
	// The configuration should also contain our own ID.
	ids := make([]hotstuff.ID, 0, h.configuration.Len())
	for id := range h.configuration.Replicas() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// Shuffle the list of IDs using the shared random seed + the first 8 bytes of the hash.
	rnd := rand.New(rand.NewSource(s.seed))
	rnd.Shuffle(len(ids), reflect.Swapper(ids))

	h.logger.Debugf("Handel session ids: %v", ids)

	s.part = newPartitioner(h.opts.ID(), ids)

	s.levels = make([]level, h.maxLevel+1)
	for i := range s.levels {
		s.levels[i] = s.newLevel(i)

		min, max := s.part.rangeLevel(i)
		h.logger.Debugf("level %d: %v", i, s.part.ids[min:max+1])
	}

	s.levels[0].individual[h.opts.ID()] = in
	s.levels[0].incoming = in

	s.updateOutgoing(1)

	s.disseminateTimerID = h.eventLoop.AddTicker(disseminationPeriod, func(_ time.Time) (event any) {
		return disseminateEvent{s.hash}
	})

	s.levelActivateTimerID = h.eventLoop.AddTicker(levelActivateInterval, func(_ time.Time) (event any) {
		return levelActivateEvent{s.hash}
	})

	return s
}

type level struct {
	vp         map[hotstuff.ID]int
	cp         map[hotstuff.ID]int
	incoming   hotstuff.QuorumSignature
	outgoing   hotstuff.QuorumSignature
	individual map[hotstuff.ID]hotstuff.QuorumSignature
	pending    []contribution
	done       bool
}

func (s *session) newLevel(i int) level {
	return level{
		vp:         verificationPriority(s.part.ids, s.seed, s.h.opts.ID(), i),
		cp:         contributionPriority(s.part.ids, s.seed, s.h.opts.ID(), i),
		individual: make(map[hotstuff.ID]hotstuff.QuorumSignature),
	}
}

func (s *session) canMergeContributions(a, b hotstuff.QuorumSignature) bool {
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

// the lock should be held when calling score.
func (s *session) score(contribution contribution) int {
	if contribution.level < 1 || int(contribution.level) > s.h.maxLevel {
		// invalid level
		return 0
	}

	level := &s.levels[contribution.level]

	need := s.part.size(contribution.level)
	if contribution.level == s.h.maxLevel {
		need = s.h.configuration.QuorumSize()
	}

	curBest := level.incoming
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
	finalParticipants := hotstuff.NewIDSet()
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

	if curBest == nil {
		total = finalParticipants.Len()
		added = total
	} else if s.canMergeContributions(contribution.signature, curBest) {
		curBest.Participants().ForEach(finalParticipants.Add)
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	} else {
		total = finalParticipants.Len()
		added = total - curBest.Participants().Len()
	}

	var score int
	if added <= 0 {
		_, haveIndiv := s.levels[contribution.level].individual[contribution.sender]
		if !haveIndiv {
			score = 1
		} else {
			score = 0
		}
	} else {
		if total == need {
			score = 1000000 - contribution.level*10 - indivAdded
		} else {
			score = 100000 - contribution.level*100 + added*10 - indivAdded
		}
	}

	s.h.logger.Debugf("level: %d, need: %d, added: %d, total: %d, score: %d", contribution.level, need, added, total, score)

	return score
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

	s.mut.Lock()
	defer s.mut.Unlock()

	score := s.score(c)
	if score < 0 {
		return
	}

	i := sort.Search(len(level.pending), func(i int) bool {
		other := level.pending[i]
		return level.vp[other.sender] <= level.vp[c.sender]
	})

	// either replace or insert a new contribution at index i
	if len(level.pending) == i || level.pending[i].sender != c.sender {
		level.pending = append(level.pending, contribution{})
		copy(level.pending[i+1:], level.pending[i:])
	}
	level.pending[i] = c

	s.h.logger.Debugf("pending contribution at level %d with score %d from sender %d", c.level, score, c.sender)

	// notify verification goroutine
	select {
	case s.newContributions <- struct{}{}:
	default:
	}
}

func (s *session) updateIncoming(c contribution) {
	s.mut.Lock()
	defer s.mut.Unlock()

	level := &s.levels[c.level]

	// check if there is a new individual signature
	if _, ok := level.individual[c.sender]; !ok {
		s.h.logger.Debugf("New individual signature from %d for level %d", c.sender, c.level)
		level.individual[c.sender] = c.signature
	}

	// check if the multisignature is an improvement
	if level.incoming != nil && c.signature.Participants().Len() <= level.incoming.Participants().Len() {
		return
	}

	s.h.logger.Debugf("New incoming aggregate signature for level %d with length %d", c.level, c.signature.Participants().Len())
	level.incoming = c.signature

	if s.isLevelComplete(c.level) {
		level.done = true
		s.advanceLevel()
		if c.level+1 <= s.h.maxLevel {
			ctx, cancel := synchronizer.ViewContext(s.h.eventLoop.Context(), s.h.eventLoop, nil)
			defer cancel()
			s.sendFastPath(ctx, c.level+1)
		}
	}

	s.updateOutgoing(c.level + 1)
}

// updateOutgoing updates the outgoing signature for the specified level,
// and bubbles up the update to the highest level.
// The lock must be held when calling this method.
func (s *session) updateOutgoing(levelIndex int) {
	if levelIndex == 0 {
		panic("cannot update the outgoing signature for level 0")
	}

	prevLevel := &s.levels[levelIndex-1]

	var (
		outgoing hotstuff.QuorumSignature
		err      error
	)

	// At level 0, there is no outgoing, so we can only use incoming.
	// However, at higher levels there may be no incoming, so we have to use outgoing.
	// It is not possible for both to be nil.
	if prevLevel.outgoing == nil {
		outgoing = prevLevel.incoming
	} else if prevLevel.incoming == nil {
		outgoing = prevLevel.outgoing
	} else {
		outgoing, err = s.h.crypto.Combine(prevLevel.incoming, prevLevel.outgoing)
		if err != nil {
			s.h.logger.Errorf("Failed to combine incoming and outgoing for level %d: %v", levelIndex, err)
			return
		}
	}

	if levelIndex > s.h.maxLevel {
		if outgoing.Participants().Len() >= s.h.configuration.QuorumSize() {
			s.h.logger.Debugf("Done with session: %.8s", s.hash)

			s.h.eventLoop.AddEvent(hotstuff.NewViewMsg{
				SyncInfo: hotstuff.NewSyncInfo().WithQC(hotstuff.NewQuorumCert(
					outgoing,
					s.h.synchronizer.View(),
					s.hash,
				)),
			})

			s.h.eventLoop.AddEvent(sessionDoneEvent{s.hash})
		}
	} else {
		level := &s.levels[levelIndex]

		level.outgoing = outgoing

		s.h.logger.Debugf("Updated outgoing for level %d: %v", levelIndex, outgoing.Participants())

		if levelIndex <= s.h.maxLevel {
			s.updateOutgoing(levelIndex + 1)
		}
	}
}

func (s *session) isLevelComplete(levelIndex int) bool {
	level := &s.levels[levelIndex]
	// check if the level is complete
	complete := true
	partition := s.part.partition(levelIndex)
	for _, id := range partition {
		if !level.incoming.Participants().Contains(id) {
			complete = false
		}
	}
	return complete
}

func (s *session) advanceLevel() {
	if s.activeLevelIndex+1 > s.h.maxLevel {
		return
	}

	s.activeLevelIndex++

	s.h.logger.Debugf("advanced to level %d", s.activeLevelIndex)
}

func (s *session) sendContributions(ctx context.Context) {
	for i := 1; i <= s.activeLevelIndex; i++ {
		if s.levels[i].done {
			continue
		}
		s.sendContributionToLevel(ctx, i)
	}
}

func (s *session) sendFastPath(ctx context.Context, levelIndex int) {
	s.h.logger.Debug("fast path activated")

	n := s.part.size(levelIndex)
	if n > 10 {
		n = 10
	}
	for i := 0; i < n; i++ {
		s.sendContributionToLevel(ctx, levelIndex)
	}
}

func (s *session) sendContributionToLevel(ctx context.Context, levelIndex int) {
	level := &s.levels[levelIndex]
	if level.outgoing == nil {
		return
	}

	part := s.part.partition(levelIndex)

	id := part[0]
	bestCP := level.cp[id]

	for _, i := range part {
		cp := level.cp[i]
		if cp < bestCP {
			bestCP = cp
			id = i
		}
	}

	if node, ok := s.h.nodes[id]; ok {
		node.Contribute(ctx, &handelpb.Contribution{
			ID:         uint32(s.h.opts.ID()),
			Level:      uint32(levelIndex),
			Signature:  hotstuffpb.QuorumSignatureToProto(level.outgoing),
			Individual: hotstuffpb.QuorumSignatureToProto(s.levels[0].incoming),
			Hash:       s.hash[:],
		}, gorums.WithNoSendWaiting())
	}

	// ensure we don't send to the same node each time
	level.cp[id] += len(s.part.ids)
}

func (s *session) verifyContributions() {
	ctx, cancel := synchronizer.ViewContext(s.h.eventLoop.Context(), s.h.eventLoop, nil)
	defer cancel()

	for ctx.Err() == nil {
		c, verifyIndiv, ok := s.chooseContribution()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.newContributions:
			}
			continue
		}
		sig := s.improveSignature(c)
		s.verifyContribution(c, sig, verifyIndiv)
	}

	s.h.eventLoop.RemoveTicker(s.disseminateTimerID)
	s.h.eventLoop.RemoveTicker(s.levelActivateTimerID)
}

// chooseContribution chooses the next contribution to verify.
// The return parameter verifyIndiv is set to true if the
// individual signature in the chosen contribution should be verified.
// The return parameter ok is set to true if a contribution was chosen,
// false if no contribution was chosen.
func (s *session) chooseContribution() (cont contribution, verifyIndiv, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()

	var choices []contribution

	// choose one contribution from each active level
	for levelIndex := 1; levelIndex <= s.activeLevelIndex; levelIndex++ {
		if s.levels[levelIndex].done {
			continue
		}
		c, ok := s.chooseContributionFromLevel(levelIndex)
		if ok {
			choices = append(choices, c)
		}
	}

	if len(choices) == 0 {
		return contribution{}, false, false
	}

	// choose the best contribution among the chosen for each level
	bestChoiceIndex := 0
	for i := range choices {
		if choices[i].score > choices[bestChoiceIndex].score {
			bestChoiceIndex = i
		}
	}

	// put the other choices back
	for choiceIndex := range choices {
		if choiceIndex == bestChoiceIndex {
			continue
		}

		level := &s.levels[choiceIndex]

		// put the choice back into its level
		pos := sort.Search(len(level.pending), func(i int) bool {
			other := level.pending[i]
			return level.vp[other.sender] < level.vp[choices[choiceIndex].sender]
		})
		level.pending = append(level.pending, contribution{})
		copy(level.pending[pos+1:], level.pending[pos:])
		level.pending[pos] = choices[choiceIndex]
	}

	best := choices[bestChoiceIndex]
	s.h.logger.Debugf("Chose: %v", best.signature.Participants())

	_, verifyIndiv = s.levels[best.level].individual[best.sender]

	return best, verifyIndiv, true
}

func (s *session) chooseContributionFromLevel(levelIndex int) (cont contribution, ok bool) {
	level := &s.levels[levelIndex]

	if len(level.pending) == 0 || level.done {
		return contribution{}, false
	}

	bestIndex := 0
	level.pending[bestIndex].score = s.score(level.pending[bestIndex])

	for i := 1; i < len(level.pending); i++ {
		score := s.score(level.pending[i])
		level.pending[i].score = score
		if i < s.window.get() && score > level.pending[bestIndex].score {
			bestIndex = i
		}
	}

	best := level.pending[bestIndex]

	var newPending []contribution
	for i, c := range level.pending {
		if c.score > 0 && i != bestIndex {
			newPending = append(newPending, c)
		}
	}

	level.pending = newPending

	if best.score == 0 {
		return contribution{}, false
	}

	return best, true
}

// improveSignature attempts to improve the signature by merging it with the current best signature, if possible,
// and adding individual signatures, if possible.
func (s *session) improveSignature(contribution contribution) hotstuff.QuorumSignature {
	level := &s.levels[contribution.level]

	signature := contribution.signature

	if s.canMergeContributions(signature, level.incoming) {
		mergedSignature, err := s.h.crypto.Combine(signature, level.incoming)
		if err == nil {
			signature = mergedSignature
		} else {
			s.h.logger.Errorf("Failed to combine signatures: %v", err)
		}
	}

	// add any individual signature, if possible
	for _, indiv := range level.individual {
		if s.canMergeContributions(signature, indiv) {
			mergedSignature, err := s.h.crypto.Combine(signature, indiv)
			if err == nil {
				signature = mergedSignature
			} else {
				s.h.logger.Errorf("Failed to combine signatures: %v", err)
			}
		}
	}

	return signature
}

func (s *session) verifyContribution(c contribution, sig hotstuff.QuorumSignature, verifyIndiv bool) {
	block, ok := s.h.blockChain.Get(s.hash)
	if !ok {
		return
	}

	s.h.logger.Debugf("verifying: %v (= %d)", sig.Participants(), sig.Participants().Len())

	aggVerified := false
	if s.h.crypto.Verify(sig, block.ToBytes()) {
		aggVerified = true
	} else {
		s.h.logger.Debug("failed to verify aggregate signature")
	}

	indivVerified := false
	// If the contribution is individual, we want to verify it separately
	if verifyIndiv {
		if s.h.crypto.Verify(c.individual, block.ToBytes()) {
			indivVerified = true
		} else {
			s.h.logger.Debug("failed to verify individual signature")
		}
	}

	indivOk := (indivVerified || !verifyIndiv)

	if indivOk && aggVerified {
		s.h.eventLoop.AddEvent(contribution{
			hash:       s.hash,
			sender:     c.sender,
			level:      c.level,
			signature:  sig,
			individual: c.individual,
			verified:   true,
		})

		s.h.logger.Debug("window increased")
		s.window.increase()
	} else {
		s.h.logger.Debugf("window decreased (indiv: %v, agg: %v)", indivOk, aggVerified)
		s.window.decrease()
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
