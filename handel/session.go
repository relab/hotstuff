package handel

import (
	"context"
	"encoding/binary"
	"fmt"
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

func canMergeContributions(a, b consensus.QuorumSignature) bool {
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

type session struct {
	// mutex is needed because pending and activeLevel.incoming
	// are accessed by verifyContributions in a separate goroutine

	mut sync.Mutex
	h   *Handel

	// static data
	hash consensus.Hash
	seed int64
	part partitioner

	// levels
	levels           []level
	activeLevel      *level
	activeLevelIndex int

	// verification
	window           window
	pending          []contribution
	newContributions chan struct{}
}

func (h *Handel) newSession(hash consensus.Hash, in consensus.QuorumSignature) *session {
	s := &session{
		h:    h,
		hash: hash,
		seed: h.mods.Options().SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:])),

		window: window{
			window:         h.mods.Configuration().Len(),
			max:            h.mods.Configuration().Len(),
			min:            2,
			increaseFactor: 2,
			decreaseFactor: 4,
		},
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
	s.activeLevel = &s.levels[0]

	s.advanceLevel()

	return s
}

type level struct {
	vp         map[hotstuff.ID]int
	cp         map[hotstuff.ID]int
	incoming   consensus.QuorumSignature
	outgoing   consensus.QuorumSignature
	individual map[hotstuff.ID]consensus.QuorumSignature
}

func (s *session) newLevel(i int) level {
	// HACK: create an empty signature as a placeholder
	emptySig, err := s.h.mods.Crypto().Combine()
	if err != nil {
		panic(fmt.Sprintf("failed to create empty signature using Combine(): %v", err))
	}

	return level{
		vp: verificationPriority(s.part.ids, s.seed, s.h.mods.ID(), i),
		cp: contributionPriority(s.part.ids, s.seed, s.h.mods.ID(), i),

		// HACK: creating empty signatures to avoid dealing with nil pointers
		incoming:   emptySig,
		outgoing:   emptySig,
		individual: make(map[hotstuff.ID]consensus.QuorumSignature),
	}
}

func (s *session) score(contribution contribution) int {
	if contribution.level < 1 || int(contribution.level) > s.h.maxLevel {
		// invalid level
		return 0
	}

	need := s.part.size(s.activeLevelIndex)
	if contribution.level == s.h.maxLevel {
		need = s.h.mods.Configuration().QuorumSize()
	}

	curBest := s.activeLevel.incoming
	if curBest.Participants().Len() >= need {
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

	for id := range s.activeLevel.individual {
		if !finalParticipants.Contains(id) {
			finalParticipants.Add(id)
			indivAdded++
		}
	}

	total := 0
	added := 0

	if canMergeContributions(contribution.signature, curBest) {
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

	score := s.score(c)
	if score < 0 {
		return
	}

	s.mut.Lock()
	defer s.mut.Unlock()

	i := sort.Search(len(s.pending), func(i int) bool {
		other := s.pending[i]
		return s.activeLevel.vp[other.sender] < s.activeLevel.vp[c.sender]
	})

	s.pending = append(s.pending, contribution{})
	copy(s.pending[i+1:], s.pending[i:])
	s.pending[i] = c

	s.h.mods.Logger().Debugf("pending contribution at level %d with score %d from sender %d", c.level, score, c.sender)

	// notify verification goroutine
	select {
	case s.newContributions <- struct{}{}:
	default:
	}
}

func (s *session) updateIncoming(c contribution) {
	s.mut.Lock()
	s.mut.Unlock()

	if c.isIndividual() {
		s.h.mods.Logger().Debugf("New individual signature from %d for level %d", c.sender, c.level)
		s.activeLevel.individual[c.sender] = c.signature
	}

	if c.signature.Participants().Len() > s.activeLevel.incoming.Participants().Len() {
		s.h.mods.Logger().Debugf("New incoming aggregate signature for level %d with length %d", c.level, c.signature.Participants().Len())
		s.activeLevel.incoming = c.signature
	}

	// check if the level is complete
	// complete := true
	// partition := s.part.partition(s.activeLevelIndex)
	// for _, id := range partition {
	// 	if !c.signature.Participants().Contains(id) {
	// 		complete = false
	// 	}
	// }

	// if complete {
	// 	s.sendFastPath()
	// }

	// check if we have enough signers for a quorum
	if s.activeLevel.incoming.Participants().Len()+s.activeLevel.outgoing.Participants().Len() >= s.h.mods.Configuration().QuorumSize() {

		sig, err := s.h.mods.Crypto().Combine(s.activeLevel.incoming, s.activeLevel.outgoing)
		if err != nil {
			s.h.mods.Logger().Errorf("Failed to combine signatures: %v", err)
			return
		}

		s.h.mods.Logger().Debugf("Done with session: %.8s", s.hash)

		s.h.mods.EventLoop().AddEvent(consensus.NewViewMsg{
			SyncInfo: consensus.NewSyncInfo().WithQC(consensus.NewQuorumCert(
				sig,
				s.h.mods.Synchronizer().View(),
				s.hash,
			)),
		})

		s.h.mods.EventLoop().AddEvent(sessionDoneEvent{s.hash})
	}
}

func (s *session) advanceLevel() {
	if s.activeLevelIndex+1 > s.h.maxLevel {
		return
	}

	out, err := s.h.mods.Crypto().Combine(s.activeLevel.outgoing, s.activeLevel.incoming)
	if err != nil {
		s.h.mods.Logger().Errorf("Failed to combine signatures: %v", err)
		return
	}

	s.activeLevelIndex++
	s.activeLevel = &s.levels[s.activeLevelIndex]

	s.activeLevel.outgoing = out
}

func (s *session) sendContributions(ctx context.Context) {
	part := s.part.partition(s.activeLevelIndex)

	id := part[0]
	bestCP := s.activeLevel.cp[id]

	for _, i := range part {
		cp := s.activeLevel.cp[i]
		if cp < bestCP {
			bestCP = cp
			id = i
		}
	}

	// TODO: can we replace this search with a map lookup instead?
	for _, node := range s.h.cfg.Nodes() {
		if node.ID() == uint32(id) {
			node.Contribute(ctx, &handelpb.Contribution{
				ID:         uint32(s.h.mods.ID()),
				Level:      uint32(s.activeLevelIndex),
				Signature:  hotstuffpb.QuorumSignatureToProto(s.activeLevel.outgoing),
				Individual: hotstuffpb.QuorumSignatureToProto(s.levels[0].outgoing),
				Hash:       s.hash[:],
			}, gorums.WithNoSendWaiting())
		}
	}

	// ensure we don't send to the same node each time
	s.activeLevel.cp[id] += len(s.part.ids)
}

func (s *session) verifyContributions(ctx context.Context) {
	for ctx.Err() == nil {
		c, sig, ok := s.chooseContribution()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.newContributions:
			}
			continue
		}
		s.verifyContribution(c, sig)
	}
}

func (s *session) chooseContribution() (cont contribution, combi consensus.QuorumSignature, ok bool) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if len(s.pending) == 0 {
		return contribution{}, nil, false
	}

	c := s.pending[0]
	bestScore := s.score(c)
	var newPending []contribution

	for i := 1; i < len(s.pending); i++ {
		score := s.score(s.pending[i])
		if i < s.window.get() && score > bestScore {
			newPending = append(newPending, c)
			c = s.pending[i]
			bestScore = score
		} else if score > 0 {
			newPending = append(newPending, s.pending[i])
		}
	}

	if bestScore == 0 {
		return contribution{}, nil, false
	}

	s.pending = newPending

	// combine with the current best signature, if possible
	sigCombi := c.signature
	if canMergeContributions(sigCombi, s.activeLevel.incoming) {
		sig, err := s.h.mods.Crypto().Combine(sigCombi, s.activeLevel.incoming)
		if err == nil {
			sigCombi = sig
		} else {
			s.h.mods.Logger().Errorf("Failed to combine signatures: %v", err)
		}
	}

	// add any individual signature, if possible
	for _, indiv := range s.activeLevel.individual {
		if canMergeContributions(sigCombi, indiv) {
			sig, err := s.h.mods.Crypto().Combine(sigCombi, indiv)
			if err == nil {
				sigCombi = sig
			} else {
				s.h.mods.Logger().Errorf("Failed to combine signatures: %v", err)
			}
		}
	}

	return c, sigCombi, true
}

func (s *session) verifyContribution(c contribution, sig consensus.QuorumSignature) {
	indivVerified := false
	// If the contribution is individual, we want to verify it separately
	if c.isIndividual() {
		if s.h.mods.Crypto().Verify(c.signature, consensus.VerifyHash(s.hash)) {
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
	if s.h.mods.Crypto().Verify(sig, consensus.VerifyHash(s.hash)) {
		aggVerified = true
		s.h.mods.EventLoop().AddEvent(contribution{
			hash:      c.hash,
			sender:    c.sender,
			level:     c.level,
			signature: sig,
			verified:  true,
		})
	}

	indivOk := (!c.isIndividual() || indivVerified)

	if indivOk && aggVerified {
		s.h.mods.Logger().Debug("window increased")
		s.window.increase()
	} else {
		s.h.mods.Logger().Debugf("window decreased (indiv: %v, agg: %v)", indivOk, aggVerified)
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
