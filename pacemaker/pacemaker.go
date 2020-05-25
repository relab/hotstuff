package pacemaker

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/relab/hotstuff"
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
	dummyCtx := context.Background()
	if p.GetID() == p.leader {
		go p.Propose(dummyCtx)
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
				go p.Propose(dummyCtx)
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

	resetTimer           chan struct{}   // sending on this channel will reset the timer
	stopTimeout          func()          // stops the new-view interrupts
	timeoutContext       context.Context // timeoutContext is passed down to the RPC's. A new context is created for every transmission.
	timeoutContextCancle func()          // timeoutContext times out after the timeout time in the pacemaker timeout variable.
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

	// set up new-view interrupt
	p.timeoutContext, p.timeoutContextCancle = context.WithCancel(ctx)
	stopContext, cancel := context.WithCancel(ctx)
	p.stopTimeout = cancel
	go p.startNewViewTimeout(stopContext)
	defer p.stopTimeout()

	// initial beat
	if p.getLeader(1) == p.GetID() {
		go p.Propose(p.timeoutContext)
	}

	// get initial notification
	n := <-notify

	// make sure that we only beat once per view, and don't beat if bLeaf.Height < vHeight
	// as that would cause a panic
	lastBeat := 1
	beat := func(ctx context.Context) {
		if p.getLeader(p.GetHeight()+1) == p.GetID() && lastBeat < p.GetHeight()+1 &&
			p.GetHeight()+1 > p.GetVotedHeight() {
			lastBeat = p.GetHeight() + 1
			go p.Propose(ctx)
		}
	}

	// handle events from hotstuff
	for {
		switch n.Event {
		case hotstuff.ReceiveProposal:
			p.resetTimer <- struct{}{}
		case hotstuff.QCFinish:
			p.timeoutContext, p.timeoutContextCancle = context.WithCancel(ctx)
			if p.GetID() != p.getLeader(p.GetHeight()+1) {
				// was leader for previous view, but not the leader for next view
				// do leader change
				go p.SendNewView(p.timeoutContext, p.getLeader(p.GetHeight()+1))
			}
			beat(p.timeoutContext)
		case hotstuff.ReceiveNewView:
			p.resetTimer <- struct{}{} // the same timeout mechanisme is used for proposal and new-view messages.
			p.timeoutContext, p.timeoutContextCancle = context.WithCancel(ctx)
			beat(p.timeoutContext)
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
		case <-p.timeoutContext.Done():
			p.timeoutContextCancle()
		case <-stopContext.Done():
			p.timeoutContextCancle()
			return
		case <-time.After(p.timeout):
			p.timeoutContextCancle()
			p.timeoutContext, p.timeoutContextCancle = context.WithCancel(context.Background()) // Would maybe be better to pass in the run context here, but whatever for now.
			// add a dummy block to the tree representing this round which failed
			logger.Println("NewViewTimeout triggered")
			p.SetLeaf(hotstuff.CreateLeaf(p.GetLeaf(), nil, nil, p.GetHeight()+1))
			p.SendNewView(p.timeoutContext, p.getLeader(p.GetHeight()+1))
		}
	}
}
