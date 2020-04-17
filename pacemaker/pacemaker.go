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
			// add a dummy node to the tree representing this round which failed
			logger.Println("NewViewTimeout triggered")
			p.SetLeafNode(hotstuff.CreateLeaf(p.GetLeafNode(), nil, nil, p.GetHeight()+1))
			p.SendNewView(p.getLeader(p.GetHeight() + 1))
		}
	}
}
