package pacemaker

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/internal/logging"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	Run(context.Context)
}

// FixedLeader uses a fixed leader.
type FixedLeader struct {
	*hotstuff.HotStuff
	leader hotstuff.ReplicaID
}

// NewFixedLeader returns a new fixed leader pacemaker
func NewFixedLeader(hs *hotstuff.HotStuff, leaderID hotstuff.ReplicaID) *FixedLeader {
	return &FixedLeader{
		HotStuff: hs,
		leader:   leaderID,
	}
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p FixedLeader) Run(ctx context.Context) {
	notify := p.GetNotifier()
	if p.GetID() == p.leader {
		go p.Propose()
	}
	var n hotstuff.Notification
	var ok bool
	for {
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
		switch n.Event {
		case hotstuff.QCFinish:
			if p.GetID() == p.leader {
				go p.Propose()
			}
		}
	}
}

// RoundRobin change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobin struct {
	*hotstuff.HotStuff

	termLength int
	schedule   []hotstuff.ReplicaID
	timeout    time.Duration

	resetTimer  chan struct{} // sending on this channel will reset the timer
	stopTimeout func()        // stops the new-view interrupts
}

// NewRoundRobin returns a new round robin pacemaker
func NewRoundRobin(hs *hotstuff.HotStuff, termLength int, schedule []hotstuff.ReplicaID, timeout time.Duration) *RoundRobin {
	return &RoundRobin{
		HotStuff:   hs,
		termLength: termLength,
		schedule:   schedule,
		timeout:    timeout,
		resetTimer: make(chan struct{}),
	}
}

// getLeader returns the fixed ID of the leader for the view height vHeight
func (p *RoundRobin) getLeader(vHeight int) hotstuff.ReplicaID {
	term := int(math.Ceil(float64(vHeight)/float64(p.termLength)) - 1)
	return p.schedule[term%len(p.schedule)]
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *RoundRobin) Run(ctx context.Context) {
	notify := p.GetNotifier()

	// initial beat
	if p.getLeader(1) == p.GetID() {
		go p.Propose()
	}

	// get initial notification
	n := <-notify

	// make sure that we only beat once per view, and don't beat if bLeaf.Height < vHeight
	// as that would cause a panic
	lastBeat := 1
	beat := func() {
		if p.getLeader(p.GetHeight()+1) == p.GetID() && lastBeat < p.GetHeight()+1 &&
			p.GetHeight()+1 > p.GetVotedHeight() {
			lastBeat = p.GetHeight() + 1
			go p.Propose()
		}
	}

	// set up new-view interrupt
	stopContext, cancel := context.WithCancel(context.Background())
	p.stopTimeout = cancel
	go p.startNewViewTimeout(stopContext)
	defer p.stopTimeout()

	// handle events from hotstuff
	for {
		switch n.Event {
		case hotstuff.ReceiveProposal:
			p.resetTimer <- struct{}{}
		case hotstuff.QCFinish:
			if p.GetID() != p.getLeader(p.GetHeight()+1) {
				// was leader for previous view, but not the leader for next view
				// do leader change
				go p.SendNewView(p.getLeader(p.GetHeight() + 1))
			}
			beat()
		case hotstuff.ReceiveNewView:
			beat()
		}

		var ok bool
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// startNewViewTimeout sends a NewView to the leader if triggered by a timer interrupt. Two contexts are used to control
// this function; the stopContext is used to stop the function, and the cancelContext is used to cancel a single timer.
func (p *RoundRobin) startNewViewTimeout(stopContext context.Context) {
	for {
		select {
		case <-p.resetTimer:
		case <-stopContext.Done():
			return
		case <-time.After(p.timeout):
			// add a dummy block to the tree representing this round which failed
			logger.Println("NewViewTimeout triggered")
			p.SetLeaf(hotstuff.CreateLeaf(p.GetLeaf(), nil, nil, p.GetHeight()+1))
			p.SendNewView(p.getLeader(p.GetHeight() + 1))
		}
	}
}

// ChangeFaulty is a pacemaker veriant which only do leader changes when a sitting leader is faulty.
// A leader is precived to be effectivly-faulty if it can't get a proposal to be approved. If a leadr can't get
// proposals certified and commited, it means either that the leader is faulty, or that the amout of faulty replicas exceeds f.
// In both cases we assume the leader to be faulty. This is a correct assumption in the first case and in the second case it dosen't realy
// matter what happens, as the system will not function what so ever if faulty replicas exceeds f.
type ChangeFaulty struct {
	*hotstuff.HotStuff
	*gorumshotstuff.HotstuffQSpec
	timeout time.Duration

	resetTimer             chan struct{}
	stopTimeout            func()
	noneFaultyReplicas     []hotstuff.ReplicaID
	isNoneFaultyReplicaMap map[hotstuff.ReplicaID]bool
}

func NewChangeFaulty(hs *hotstuff.HotStuff, qs *gorumshotstuff.HotstuffQSpec, noneFaultyReplicas []hotstuff.ReplicaID, timeout time.Duration) *ChangeFaulty {
	p := &ChangeFaulty{
		HotStuff:               hs,
		HotstuffQSpec:          qs,
		timeout:                timeout,
		resetTimer:             make(chan struct{}),
		noneFaultyReplicas:     noneFaultyReplicas,
		isNoneFaultyReplicaMap: make(map[hotstuff.ReplicaID]bool),
	}

	for _, id := range p.noneFaultyReplicas {
		p.isNoneFaultyReplicaMap[id] = true
	}

	logger.Println(noneFaultyReplicas)

	return p
}

func (p *ChangeFaulty) isNoneFaulty(id hotstuff.ReplicaID) bool {
	return p.isNoneFaultyReplicaMap[id]
}

func (p *ChangeFaulty) getLeader() hotstuff.ReplicaID {
	return p.noneFaultyReplicas[0]
}

// updateNoneFaultySlice updates state regarding faulty replicas. It uses the latest replies from the qspec.
// This should probably only be called by the leader as the replies from the qspec from a none leader replica can be very old and outdated.
// Updating the the schdual should probably not be done unless there are some form of reconfiguration or chatch up mechanism. As of now, a new replica can't
// be added. If a replica is faulty and stop being faulty, we don't have code for handeling it anyway.
func (p *ChangeFaulty) upadteNoneFaultySlice() {
	_, replies := p.HotstuffQSpec.GetLatestReplies()
	for id, pc := range replies {
		if pc != nil && !p.isNoneFaulty(hotstuff.ReplicaID(id)) {
			p.isNoneFaultyReplicaMap[hotstuff.ReplicaID(id)] = true
			p.noneFaultyReplicas = append(p.noneFaultyReplicas, hotstuff.ReplicaID(id)) // There could potentialy be a double check of the contetns of this list. There should be no duplicat IDs.
		}
	}
}

func (p *ChangeFaulty) setFaulty(id hotstuff.ReplicaID) {
	if p.isNoneFaulty(id) {
		p.isNoneFaultyReplicaMap[id] = false
		if len(p.noneFaultyReplicas) < 1 {
			p.noneFaultyReplicas = []hotstuff.ReplicaID{}
		} else {
			p.noneFaultyReplicas = p.noneFaultyReplicas[1:]
		}
	}
	logger.Println(p.noneFaultyReplicas)
}

// A problem with this sort of pacemaker is that there is no guarantee for new leader to be none-fualty. Only the leader can obtain new information form the qspec,
// which is crutial if a replica want to know if other replicas are faulty or not. Normal replicas can only figure out wheter or not the leader is faulty, as normal
// replicas only talk to the leader and not other normal replicas. The CangeFaulty pacemaker changes a leader only if it becomes faulty, but does not guarantee that a
// new none-faulty leader will be elected. The leade replica would konw the condtion of other normal replicas, but this is of litle use in this pacemaker as the leader
// does not become a normal replica, unless it is faulty and then recovers. This is not possible as of now. A sugestion for a pacemaker that can guarantee that a
// faulty leader is never(or atleast almost never) elected, would be a modified RR pacemaker. If all replicas are often leader, they would be able to know about the
// condition of other rplicas.

// Run starts the pacemaker.
func (p *ChangeFaulty) Run(ctx context.Context) {
	notify := p.GetNotifier()
	if p.getLeader() == p.GetID() {
		go p.Propose()
	}
	n := <-notify

	lastBeat := 1
	beat := func() {
		if p.getLeader() == p.GetID() && lastBeat < p.GetHeight()+1 &&
			p.GetHeight()+1 > p.GetVotedHeight() {
			lastBeat = p.GetHeight() + 1
			go p.Propose()
		}
	}

	stopContext, cancel := context.WithCancel(context.Background())
	p.stopTimeout = cancel
	go p.startNewViewTimeout(stopContext)
	defer p.stopTimeout()

	newViewQuorum := 0

	for {
		switch n.Event {
		case hotstuff.ReceiveProposal: // it would have been nice if some where given regarding who sent the proposal.
			p.resetTimer <- struct{}{}
		case hotstuff.QCFinish:
			if n.QC == nil {
				p.setFaulty(p.getLeader())
				go p.SendNewView(p.getLeader())
			}
			beat()
		case hotstuff.ReceiveNewView:
			newViewQuorum++
			if p.getLeader() == p.GetID() && p.HotStuff.QuorumSize <= newViewQuorum {
				newViewQuorum = 0
				beat()
			} else if p.HotStuff.QuorumSize <= newViewQuorum {
				newViewQuorum = 0
				p.setFaulty(p.getLeader())
				beat()
			}

		}

		var ok bool
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *ChangeFaulty) startNewViewTimeout(stopContext context.Context) {
	for {
		select {
		case <-p.resetTimer:
		case <-stopContext.Done():
			return
		case <-time.After(p.timeout):
			// add a dummy block to the tree representing this round which failed
			logger.Println("NewViewTimeout triggered")
			p.SetLeaf(hotstuff.CreateLeaf(p.GetLeaf(), nil, nil, p.GetHeight()+1))
			p.setFaulty(p.getLeader())
			p.SendNewView(p.getLeader())
		}
	}
}

// GoodLeader is a pacemaker which (almost)guarantee that a none-faulty replica is chosen as the leader for any given view.
// The premice of how it works are the following: The leader alone is responsible for picking a good new candiate as the next leader.
// The leader is changed every view to reduce the risk of the leader sudenly becoming faulty. When a leader elect a new leader, it cannot
// elect it self to avoid a "tyrant" leader, however the newly elected leader can elect the privous leader after it's term. This means
// that two leaders can always elect eachother and not let any other replicas lead, but this is fine. If a timeout occures, all replicas send a
// new-view message to the privous leader which led the last successfull proposal round, as that is the best leader at the time timout which the
// normal replicas know of. When reciving a new proposal notification an ID of who sent the proposal is given along with the proposal. This is
// how normal replicas know who the current leader is, and they keep track of who the leader where two rounds ago. There is no schedule in use.
// If a leader elect a new replica as the leader the replica just start to beat, although it checks that the incoming message truly is from the last leader.
// In the rear case of a timeout that times out, meaing that the leader chosen by the normal replicas in the timout handler is also faulty, there are new
// atempts of electing a leader. This election process just uses a list of replicaID's which is the same for every replica. If a replica recives new-view messages
// not from the leader, it waits for a qourm amount of messages before beating. In the event of a leader being faulty, but not causing a timeout, a random
// replica is elected as the new leader. This is because a faulty leader can not be trusted to figure out who should be leader next. If a faulty leader
// got to elect a new leader by itself, it could choose another faulty leader as the new leader. The new leader could choose the old faulty leader and so on.
//
// The leader rely on the Qspec to figure out who the best leader candidate is. The Qspec select a replica that where apart of the signing of the current QC and which
// where the first to respond to the proposal.
type GoodLeader struct {
	*hotstuff.HotStuff
	*gorumshotstuff.HotstuffQSpec
	timeout time.Duration

	resetTimer             chan struct{}
	stopTimeout            func()
	schedule               []hotstuff.ReplicaID
	termLength             int
	isNoneFaultyReplicaMap map[hotstuff.ReplicaID]bool
	lastTwoLeaders         [2]hotstuff.ReplicaID
}

func NewGoodLeader(hs *hotstuff.HotStuff, qs *gorumshotstuff.HotstuffQSpec, schedule []hotstuff.ReplicaID, timeout time.Duration) *GoodLeader {
	p := &GoodLeader{
		HotStuff:               hs,
		HotstuffQSpec:          qs,
		timeout:                timeout,
		resetTimer:             make(chan struct{}),
		schedule:               schedule,
		termLength:             1,
		isNoneFaultyReplicaMap: make(map[hotstuff.ReplicaID]bool),
	}

	p.lastTwoLeaders = [2]hotstuff.ReplicaID{0, 0}

	for _, id := range schedule {
		p.isNoneFaultyReplicaMap[id] = true
	}

	return p
}

func (p *GoodLeader) getOldLeader() hotstuff.ReplicaID {
	return p.lastTwoLeaders[1]
}

func (p *GoodLeader) updateOldLeader(newID hotstuff.ReplicaID) {
	p.lastTwoLeaders[1] = p.lastTwoLeaders[0]
	p.lastTwoLeaders[0] = newID
}

func (p *GoodLeader) evictLeader() {
	p.lastTwoLeaders[0] = p.lastTwoLeaders[1]
}

func (p *GoodLeader) getLeader(vHeight int) hotstuff.ReplicaID {
	term := int(math.Ceil(float64(vHeight)/float64(p.termLength)) - 1)
	return p.schedule[term%len(p.schedule)]
}

func (p *GoodLeader) Run(ctx context.Context) {
	notify := p.GetNotifier()

	if p.getLeader(1) == p.GetID() {
		go p.Propose()
	}

	n := <-notify

	lastBeat := 1
	beat := func() {
		if lastBeat < p.GetHeight()+1 && p.GetHeight()+1 > p.GetVotedHeight() {
			lastBeat = p.GetHeight() + 1
			go p.Propose()
		}
	}

	stopContext, cancel := context.WithCancel(context.Background())
	p.stopTimeout = cancel
	go p.startNewViewTimeout(stopContext)
	defer p.stopTimeout()

	qCounter := 0

	for {
		switch n.Event {
		case hotstuff.ReceiveProposal:
			p.resetTimer <- struct{}{}
			p.updateOldLeader(n.SenderID)
			qCounter = 0
		case hotstuff.QCFinish:
			logger.Println(p.HotstuffQSpec)
			go p.SendNewView(p.HotstuffQSpec.GetBestReplica())
			p.HotstuffQSpec.FlushBestReplica()
		case hotstuff.ReceiveNewView:
			qCounter++
			if n.SenderID == p.lastTwoLeaders[0] {
				qCounter = 0
				beat()
			} else if p.HotStuff.QuorumSize <= qCounter {
				qCounter = 0
				beat()
			}
		}

		var ok bool
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *GoodLeader) startNewViewTimeout(stopContext context.Context) {
	for {
		select {
		case <-p.resetTimer:
		case <-stopContext.Done():
			return
		case <-time.After(p.timeout):
			logger.Println("NewViewTimeout triggered")
			p.SetLeaf(hotstuff.CreateLeaf(p.GetLeaf(), nil, nil, p.GetHeight()+1))
			if p.getOldLeader() == 0 || p.getOldLeader() == p.lastTwoLeaders[0] {
				p.SendNewView(p.getLeader(p.GetHeight() + 1))
			} else {
				p.evictLeader()
				p.SendNewView(p.getOldLeader())
			}

		}
	}
}
